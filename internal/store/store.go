// Package store é a camada de persistência SQLite (modernc.org/sqlite, Go puro,
// sem CGO) da biblioteca: bibliotecas em árvore, livros e progresso de leitura.
//
// Concorrência: o scanner grava lotes (UpsertBooks) enquanto a UI lê
// (ListBooks, Stats) e grava progresso (SaveProgress). Leituras usam o pool do
// database/sql direto (o modo WAL deixa ler durante uma escrita). Toda escrita,
// inclusive a migração e o "lê e depois escreve" do SaveProgress, passa por um
// sync.Mutex do Store: sem ele, uma transação que lê e depois grava falha com
// SQLITE_BUSY quando há outro escritor, mesmo com busy_timeout (medido no spike
// da Etapa 4: 925 de 994 falhas sem o mutex, 0 com ele). Não limitamos o pool a
// uma conexão (SetMaxOpenConns(1)) porque isso deixou as leituras 4 a 6 vezes
// mais lentas.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // driver "sqlite", Go puro
)

// SchemaVersion é a versão do esquema que este código cria e entende
// (PRAGMA user_version).
const SchemaVersion = 1

var (
	// ErrSchemaTooNew indica um banco criado por uma versão mais nova do app.
	// O arquivo não é alterado.
	ErrSchemaTooNew = errors.New("store: o banco de dados foi criado por uma versão mais nova do aplicativo")
	// ErrInvalidTree indica uma biblioteca que deixaria a árvore inconsistente
	// (ciclo, pai inexistente/próprio, raiz incoerente). Nada é gravado.
	ErrInvalidTree = errors.New("store: árvore de bibliotecas inválida")
	// ErrNotFound indica que o registro pedido não existe.
	ErrNotFound = errors.New("store: não encontrado")
)

// maxTreeDepth limita a profundidade percorrida em consultas e validações
// recursivas da árvore de bibliotecas. Uma árvore de pastas real fica muito
// abaixo disso; o limite só existe para que um ciclo (banco editado à mão ou
// bug futuro) nunca faça uma consulta girar para sempre.
const maxTreeDepth = 256

// pragmas valem para toda conexão aberta pelo pool (o driver os aplica ao
// abrir cada conexão).
const dsnPragmas = "_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

// Store é o acesso ao banco. É seguro para uso concorrente.
type Store struct {
	db *sql.DB
	mu sync.Mutex // serializa toda escrita (ver a doc do pacote)
}

// Open abre (criando, se preciso, o arquivo e a pasta-mãe) o banco em path,
// aplica os pragmas e migra o esquema para SchemaVersion. Um banco com
// user_version maior devolve ErrSchemaTooNew sem tocar no arquivo.
func Open(path string) (*Store, error) {
	// A DSN é o caminho puro, SEM o prefixo "file:": com "file:" um "#" na
	// pasta vira fragmento de URI e o banco é criado no lugar errado, em
	// silêncio (spike da Etapa 4). O "?" separa a query de pragmas, então não
	// pode aparecer no caminho.
	if strings.Contains(path, "?") {
		return nil, fmt.Errorf("store: caminho do banco não pode conter '?': %q", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("store: criar pasta do banco: %w", err)
	}

	// Pré-checagem de versão num banco já existente, ANTES de aplicar o pragma
	// journal_mode(WAL) (que reescreve o cabeçalho do arquivo se ele não
	// estiver em WAL): um banco de versão futura não pode ser alterado.
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		v, err := peekUserVersion(path)
		if err != nil {
			return nil, err
		}
		if v > SchemaVersion {
			return nil, fmt.Errorf("%w (esquema %d, este app entende até %d)", ErrSchemaTooNew, v, SchemaVersion)
		}
	}

	db, err := sql.Open("sqlite", path+"?"+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("store: abrir banco: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// peekUserVersion lê o user_version sem aplicar pragmas que escrevam.
func peekUserVersion(path string) (int, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return 0, fmt.Errorf("store: abrir banco: %w", err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("store: ler versão do esquema: %w", err)
	}
	return v, nil
}

// Close fecha o banco, esperando uma escrita em andamento terminar.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// write executa fn numa transação, sob o mutex de escrita. Faz commit se fn
// devolver nil e rollback caso contrário.
func (s *Store) write(ctx context.Context, fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(ctx, fn)
}

// writeLocked é write para quem já segura s.mu (a migração).
func (s *Store) writeLocked(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// naturalLess compara nomes sem diferenciar maiúsculas e tratando sequências
// de dígitos como números ("HQ 2" antes de "HQ 10"). Mesma ideia do
// naturalLess de internal/archive, que não é importado de propósito.
func naturalLess(a, b string) bool {
	return naturalCompare(a, b) < 0
}

func naturalCompare(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			ei, ej := i, j
			for ei < len(a) && isDigit(a[ei]) {
				ei++
			}
			for ej < len(b) && isDigit(b[ej]) {
				ej++
			}
			// Compara como número sem converter (sem estouro): tira zeros à
			// esquerda, o mais comprido é maior, depois lexical.
			na := strings.TrimLeft(a[i:ei], "0")
			nb := strings.TrimLeft(b[j:ej], "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			i, j = ei, ej
			continue
		}
		if ca != cb {
			if ca < cb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case len(a)-i < len(b)-j:
		return -1
	case len(a)-i > len(b)-j:
		return 1
	}
	return 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
