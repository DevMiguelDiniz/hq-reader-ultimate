# Leitor de HQs — Design v1

Data: 2026-09-16
Status: aprovado para plano de implementação

## 1. Objetivo

Aplicativo desktop nativo (Windows, Linux, macOS) para leitura de CBZ e CBR.
Requisitos: leve, compilável/instalável (sem execução via terminal), UI moderna.
Mobile fora de escopo.

## 2. Stack

| Camada | Escolha |
|---|---|
| Framework desktop | Wails v3 (beta, pin em versão fixa, não `@latest`) |
| ZIP (CBZ) | `archive/zip` (stdlib) |
| RAR (CBR) | `nwaples/rardecode` (Go puro) |
| Resize de thumbnail | `golang.org/x/image/draw` (CatmullRom) |
| Metadados | `encoding/xml` (stdlib), `ComicInfo.xml` |
| Ordenação natural | implementação própria ou `maruel/natural` |
| Persistência | SQLite via `mattn/go-sqlite3` (CGO — já exigido pelo Wails no macOS/Linux; confirmar se Windows também exige antes da Etapa 4) |
| Frontend | Vite + React + TypeScript, Tailwind, shadcn/ui, Zustand, Motion |

Go foi escolhido sobre Rust por causa do RAR: em Go, `rardecode` resolve CBR sem
CGO e sem conflito de licença; em Rust a alternativa seria libarchive (dependência
C) ou o crate `unrar` (licença incompatível). Trade-off aceito: `image/jpeg` da
stdlib é mais lento que libjpeg-turbo, mas isso só afeta geração de thumbnail —
quem decodifica página cheia é o WebView, não o Go.

## 3. Arquitetura

### 3.1 Decisão central: páginas servidas por HTTP, não por IPC

O `AssetServer` do Wails expõe uma rota `/page/{bookID}/{n}` que faz `io.Copy`
dos bytes crus da imagem para o `ResponseWriter`. O front usa
`<img src="/page/{bookID}/{n}">`. O WebView decodifica com o decoder nativo do
SO — sem re-encode, sem base64, sem cópia intermediária.

Regras de segurança e cache:

- `bookID` é um UUID de sessão gerado no `Open()`, **nunca** o caminho do
  arquivo — evita que a rota vire uma superfície de leitura arbitrária de disco.
- Resposta leva `Cache-Control: immutable` (o conteúdo de um `bookID+n` nunca
  muda durante a sessão), evitando refetch do WebView em re-render.
- Content-Type e Content-Length do `Archive.Open()` são conhecidos antes do
  `io.Copy`, para evitar transfer-encoding chunked.

Validação obrigatória antes de qualquer UI (Etapa 0): confirmar que o Wails v3
permite registrar essa rota no mesmo `AssetServer` da SPA.

### 3.2 Services sem dependência de janela

Toda a lógica de negócio vive em services (`LibraryService`, `ReaderService`,
`SettingsService`, `WindowService`) que não conhecem `Window`. Isso sustenta
duas decisões da v1:

- Janela única que pode virar múltiplas janelas destacáveis (Etapa 7): o estado
  (livro aberto, página atual) precisa estar no backend desde a Etapa 3, nunca
  só em `useState` do React, senão o retrofit da Etapa 7 exige reescrever a
  Etapa 3.
- Mantém aberta (sem implementar agora) a opção de um server build futuro
  (biblioteca acessível via navegador).

### 3.3 Interface Archive

```go
type PageEntry struct {
    Index       int
    Size        int64
    ContentType string
}

type Archive interface {
    Pages() []PageEntry           // já ordenado naturalmente
    Open(index int) (io.ReadCloser, error)
    Metadata() (*ComicInfo, error)
    Close() error
}
```

`PageEntry.Size`/`ContentType` existem para o handler HTTP poder setar
`Content-Length` sem bufferizar a página inteira antes de responder.

Assimetria entre implementações:

- `zipArchive`: acesso aleatório direto, sem estado extra.
- `rarArchive`: `rardecode` é sequencial. RAR **sólido** (comum em releases de
  scan) comprime entre arquivos — ler a página 80 exige decodificar tudo antes
  dela, sempre. Por isso `rarArchive.Open()` na primeira chamada extrai o
  arquivo inteiro para um diretório temporário e serve dali em diante. Isso é
  potencialmente lento e será medido na Etapa 0 com amostras reais de RAR
  sólido — se o tempo for inaceitável, a UI precisa de indicador de progresso
  na abertura (decisão adiada até medir).

Ciclo de vida do diretório temporário do `rarArchive`:

1. Extração lazy, no primeiro `Open()` do livro — não no momento em que o
   usuário só seleciona o livro na biblioteca.
2. Caminho: `os.UserCacheDir()/hqreader/tmp-extract/<bookID>/`.
3. Limpeza no `Close()` explícito do livro, e também registrada no shutdown do
   app.
4. No **startup** do app, antes de qualquer outra coisa, varrer e apagar
   qualquer `tmp-extract/*` órfão de uma sessão anterior encerrada por crash.

### 3.4 Fluxo de dados: abrir e ler um livro

1. Biblioteca → `LibraryService` resolve o caminho → `ReaderService.Open(path)`
   monta `zipArchive` ou `rarArchive` conforme extensão → registra sob um
   `bookID` de sessão → retorna contagem de páginas e metadados ao front.
2. Front renderiza `<img src="/page/{bookID}/{n}">` para a página atual. Não há
   prefetch no front — o backend decide o que já está pronto em RAM.
3. Um **prefetch coordinator** (uma goroutine por livro aberto) escuta o evento
   "página atual mudou" (chamada explícita do front a cada navegação), calcula
   a janela N±2, cancela via `context.Context` os fetches que saíram da janela
   e popula um ring buffer de 5 slots com os bytes já lidos da entrada do
   archive.
4. O handler HTTP checa o ring buffer primeiro (hit = cópia de RAM); em miss,
   lê direto do archive (cache-through) e serve do mesmo jeito.

Um coordinator por livro **aberto** (não por livro da biblioteca) — com a
Etapa 7, pode haver dois coordinators simultâneos, sem estado compartilhado
entre si.

### 3.5 Modelo de dados (SQLite, `mattn/go-sqlite3`, `journal_mode=WAL`)

```sql
CREATE TABLE books (
  id          TEXT PRIMARY KEY,          -- hash(path), atribuído na inserção
  path        TEXT NOT NULL UNIQUE,
  size        INTEGER NOT NULL,
  mtime       INTEGER NOT NULL,
  format      TEXT NOT NULL,             -- 'cbz' | 'cbr'
  page_count  INTEGER,
  title       TEXT,
  added_at    INTEGER NOT NULL,
  missing     INTEGER NOT NULL DEFAULT 0,  -- path sumiu num rescan
  broken      INTEGER NOT NULL DEFAULT 0   -- falhou validação rasa no scan
);

CREATE TABLE collections (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE collection_books (
  collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  book_id       TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  position      INTEGER,
  PRIMARY KEY (collection_id, book_id)
);

CREATE TABLE reading_progress (
  book_id      TEXT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  current_page INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL
);

CREATE TABLE book_settings (
  book_id   TEXT PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
  direction TEXT NOT NULL DEFAULT 'ltr',    -- 'ltr' | 'rtl' (mangá — fora da v1)
  layout    TEXT NOT NULL DEFAULT 'paged'   -- 'paged' | 'continuous' (fora da v1)
);

CREATE TABLE thumbs (
  key         TEXT PRIMARY KEY,   -- hash(path + mtime + size + page)
  data        BLOB NOT NULL,
  bytes       INTEGER NOT NULL,
  accessed_at INTEGER NOT NULL
);
CREATE INDEX idx_accessed ON thumbs(accessed_at);
```

Notas de design do schema:

- `id` do livro é hash do **caminho**, não do conteúdo — um hash de conteúdo
  completo para 500 arquivos grandes é caro demais para rodar em todo rescan.
  Se o arquivo for movido, a linha vira `missing=1`; não há detecção automática
  de movimentação — o usuário readiciona manualmente. Complexidade de matching
  por conteúdo não compensa o retorno aqui.
- `book_settings` fica separado de `reading_progress` porque progresso muda a
  cada virada de página (escrita frequente, deve ser debounced) e settings
  mudam raramente.
- `thumbs.key` inclui `mtime`+`size`: se o usuário substituir o CBZ por outra
  versão no mesmo caminho, a chave muda sozinha e o thumb velho nunca é servido
  por engano.
- Atualizar `accessed_at` em toda leitura de thumb seria uma escrita a cada
  hit — evitar contenção fazendo throttling (só grava se a última atualização
  tiver mais de alguns minutos).
- Limpeza de cache por LRU usa `PRAGMA auto_vacuum=INCREMENTAL` +
  `incremental_vacuum` periódico, não `VACUUM` completo a cada limpeza (caro,
  reescreve o arquivo inteiro).
- Teto de cache configurável, padrão ~500 MB, derruba por `accessed_at` até
  voltar a 80%. Tela de configuração mostra tamanho atual e permite limpeza
  manual.

Regras de cache de página (não mudou do briefing original):

- Nunca cachear página em tamanho cheio em disco.
- Capa sempre cacheada; thumbs de página sob demanda (grade aberta, só as
  visíveis).

### 3.6 Tratamento de erro

- Erros de archive são sentinelas tipadas (`ErrPasswordProtected`,
  `ErrCorrupted`, `ErrUnsupportedFormat`), propagadas ao front como código, não
  string — a UI decide ícone/mensagem sem parsear texto.
- Falha ao servir uma página específica: handler HTTP retorna HTTP 500; o
  `<img onError>` no front troca para um asset local de "página corrompida",
  sem derrubar a leitura das demais páginas do livro.
- Scan de biblioteca: arquivo sumiu → `missing=1`. Falhou validação rasa (não
  lista entradas) → `broken=1`. Nenhum dos dois casos interrompe o scan dos
  demais livros. Validação rasa não decodifica páginas — corrupção no meio do
  arquivo ainda pode aparecer só na leitura.

### 3.7 Concorrência

- Um prefetch coordinator por livro aberto, dono exclusivo do seu ring buffer.
- `*sql.DB` único em modo WAL — leituras concorrentes livres, escritas
  serializadas pelo próprio SQLite.
- Geração de thumbnail: worker pool limitado (tamanho = `NumCPU`), fila
  alimentada pelas thumbs visíveis no grid — não spawna goroutine por thumb ao
  rolar rápido.

## 4. Decisões fechadas

| Item | Decisão | Justificativa |
|---|---|---|
| Driver SQLite | `mattn/go-sqlite3` | Wails já exige CGO no macOS/Linux; portabilidade pure-Go não paga o custo de performance |
| Modo contínuo/webtoon | Fora da v1, planejado (schema já suporta via `book_settings.layout`) | Puramente front-end sobre a mesma rota HTTP; sem custo de retrofit |
| Modo mangá (RTL) | Fora da v1, planejado (schema já suporta via `book_settings.direction`) | Mesma lógica do item acima |
| Agrupamento de biblioteca | Coleções criadas manualmente pelo usuário (não auto-detecção de série) | Requisito explícito do usuário |
| Modelo de janela | Janela única na v1, arquitetura pronta para destacar (Etapa 7) | Estado já vive nos services desde a Etapa 3 |
| Auto-update | Nenhum na v1 — distribuição via download manual (GitHub Releases) | Assinatura de código e pipeline de release não se justificam antes de uma base de usuários |
| Subpastas internas do arquivo | Achatar e ordenar por caminho relativo completo | Tratar subpasta como capítulo é feature maior, adiada |
| Páginas duplas (spread) | Detectar por proporção; não dividir automaticamente; no modo duplo, mostrar sozinha | Dividir programaticamente exige achar a costura — caro e frágil |
| Entradas não-imagem no archive | Filtrar por extensão (allowlist), nunca por posição | `Thumbs.db`, `.DS_Store`, `ComicInfo.xml` não são páginas |
| Formatos de imagem suportados na v1 | JPEG, PNG, WebP (decode nativo do WebView para página cheia; `x/image` para thumbnail) | AVIF/JXL sem encoder Go viável para thumbnail — mostrar placeholder de erro por página em vez de tentar suportar |
| Watcher de biblioteca | Nenhum na v1 — rescan manual | `fsnotify` recursivo em biblioteca grande tende a dar mais problema que resolve |

## 5. Riscos e validação (Etapa 0 — antes de qualquer código de produto)

Cada item abaixo é um go/no-go que pode mudar a arquitetura antes dela virar
produto:

1. Rota HTTP custom registrável no `AssetServer` do Wails v3 ao lado da SPA.
2. Custo real (tempo, disco, memória) de extrair um CBR sólido de amostra.
3. Pan/zoom por `transform` numa imagem de 3000px no WebKitGTK (Linux) — fluido
   ou não. Se não for, decidir mitigação (downscale de exibição, `will-change`,
   renderização por canvas) antes da Etapa 3.
4. Wails v3 permite spawnar/fechar janela em runtime compartilhando estado do
   backend (valida a viabilidade da Etapa 7 antes de desenhar em cima dela).
5. Confirmar se o build do Wails v3 para Windows exige CGO (WebView2 costuma
   ser via COM/syscall). Se não exigir, é uma inconsistência a decidir
   conscientemente antes da Etapa 4 (driver SQLite já escolhido pressupõe CGO
   disponível em todas as plataformas de build).

Artefato desta etapa: relatório curto com o resultado de cada item, código
descartado ao final (não entra na base do produto).

## 6. Roteiro de etapas (cada uma com artefato demonstrável)

| Etapa | Entrega | Artefato de validação |
|---|---|---|
| 0 | Spikes de risco (§5) | Relatório + PoCs descartáveis |
| 1 | Núcleo `Archive` (Go puro) | `hqcli list`/`hqcli dump` + testes contra `testdata/` (RAR sólido, senha, truncado, subpasta, não-imagem) |
| 2 | Página servida por HTTP num shell Wails mínimo | App de uma janela, `<img src="/page/...">` + próxima/anterior, sem estilo |
| 3 | Leitor completo de um arquivo | Zoom/pan, página única/dupla, prefetch, atalhos, placeholder de erro por página; estado do livro aberto já no backend |
| 4 | Biblioteca e scanner | Apontar pasta, escanear, grid plano, abrir na Etapa 3, rescan manual |
| 5 | Coleções | Criar/nomear coleção, atribuir livros, grid agrupado |
| 6 | Thumbnails e cache | Grid com capa/thumbs sob demanda, LRU, tela de config com tamanho + limpeza |
| 7 | Janela destacável | Leitor ou biblioteca vira janela OS separada e volta a acoplar, sem perder estado |
| 8 | Empacotamento | Instalador por plataforma (NSIS/MSI, dmg, AppImage/deb), sem assinatura de código |

## 7. Testes

- Etapa 1: testes table-driven contra fixtures sintéticas em `testdata/` (sem
  HQs com copyright), cobrindo RAR sólido, senha, truncado, subpasta, entrada
  não-imagem.
- Etapa 4+: testes de unidade do scanner contra filesystem fake; testes de
  service com store mockado.
- Etapa 2/3: verificação manual guiada pelos achados da Etapa 0. Sem cobertura
  automatizada de UI na v1 — reavaliar se o projeto crescer.

## 8. Fora de escopo da v1 (mas com gancho arquitetural pronto)

- Modo de leitura contínuo/webtoon (`book_settings.layout`).
- Modo mangá / RTL (`book_settings.direction`).
- Auto-update assinado.
- Detecção automática de arquivo movido (move-detection por conteúdo).
- Watcher de biblioteca (`fsnotify`).
- Suporte a AVIF/JXL.
- Server build (biblioteca acessível via navegador) — não implementado, mas os
  services já são window-agnostic para não inviabilizar depois.

## 9. Próximo passo

Este documento cobre o design de arquitetura completo da v1. O detalhamento de
tarefas por etapa (começando pela Etapa 0) é feito via plano de implementação
(skill `writing-plans`), não neste documento.
