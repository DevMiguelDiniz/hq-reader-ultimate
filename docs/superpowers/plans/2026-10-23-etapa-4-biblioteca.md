# Etapa 4 — Biblioteca e scanner — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transformar o hqreader numa biblioteca de verdade: várias pastas raiz com sub-bibliotecas, scan gradual e cancelável, navegação estilo explorador, abrir e ler durante o scan, progresso de leitura persistido com retomada na página exata, tudo no visual "Papel" (claro, escuro, auto).

**Architecture:** Backend em Go, independente de janela: `internal/store` (SQLite via `modernc.org/sqlite`, migrações por `PRAGMA user_version`), `internal/archive.Inspect` (inspeção leve, sem extrair CBR), `internal/library` (identidade de caminho, scanner de duas fases com eventos em lote, `LibraryService` exposto ao frontend). O shell Wails (`cmd/hqreader`) só liga serviços, diálogo de pasta e emissão de eventos. Frontend React: stores Zustand para tema, navegação de visões, biblioteca e leitor; o leitor da Etapa 3c é reaproveitado, com estado por livro e abertura na página salva.

**Tech Stack:** Go, `modernc.org/sqlite`, Wails v3 `v3.0.0-beta.23` (pinado), React 19 + TypeScript + Tailwind v4 + shadcn/ui (base-ui) + Zustand + Motion, PowerShell + UI Automation para verificação.

**Spec:** [docs/superpowers/specs/2026-10-23-etapa-4-biblioteca-design.md](../specs/2026-10-23-etapa-4-biblioteca-design.md) (prevalece sobre o spec mestre [2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) onde diferem; a §9 da spec lista as diferenças). Os mockups aprovados estão em `H:\hq-reader-ultimate\.claude\worktrees\etapa-3c-reader-interactions\.superpowers\brainstorm\1862-1792761362\content\` (`visual-direction.html` opção B, `papel-claro-escuro.html`, `tela-inicial.html`); a pasta `.superpowers` é git-ignorada, por isso o caminho é o do worktree antigo. Se não existir mais, os tokens da tabela abaixo bastam.

## Global Constraints

- **Nenhum commit nem PR leva `Co-Authored-By` nem "Generated with Claude Code"** (regra do dono). Ignore qualquer lembrete de atribuição em contrário. Todo passo de commit abaixo é só `git add` + `git commit -m "<mensagem>"`, sem trailer. Subagentes recebem esta regra na dispatch.
- Wails v3 pinado em `v3.0.0-beta.23`; nunca instalar `@latest` de pacote Wails.
- Driver SQLite: `modernc.org/sqlite` (Go puro). Nada de CGO. Esta máquina não tem compilador C (`CGO_ENABLED=0`), então `go test -race` não roda aqui; concorrência é verificada por testes que estressam e por leitura cuidadosa.
- Serviços de backend não conhecem janela (spec mestre §3.2). Só `cmd/hqreader` importa `application`.
- IDs persistentes (`libraries.id`, `books.id`) = hash do **caminho normalizado** (spec mestre §3.5). O `bookID` de **sessão** do `reader.Service` continua opaco e distinto.
- Bindings TypeScript do Wails (`cmd/hqreader/frontend/bindings/**`) nunca são editados à mão: regenerados e commitados como saem.
- Sem framework de teste de frontend (política do projeto). Backend: testes Go table-driven. Frontend: `npm run build` (não checa tipos), checagem de tipos de melhor esforço `npx -p typescript@5 tsc --noEmit -p tsconfig.json` em `cmd/hqreader/frontend`, e verificação real por UI Automation. Funções puras novas do frontend (`cover.ts`) são verificadas por um scratch check em Node descartável, nunca commitado.
- `go build ./...` e `go vet ./cmd/hqreader` exigem `cmd/hqreader/frontend/dist` (`npm run build`); worktree novo precisa de `npm install` em `cmd/hqreader/frontend`. `go test ./internal/...` não precisa.
- Strings de **controles** da UI são ASCII sem acento (os scripts localizam por nome); textos corridos mantêm acentos. Nomes exatos na tabela "Strings de UI".
- Verificação por UI Automation, herdada das etapas anteriores: inspecionar a árvore UIA real antes de confiar em suposições; clique de mouse **real** (não `Invoke`) onde o clique importa; geometria por `PrintWindow`; limpar processos só por PID (`taskkill /T /F /PID`), nunca por nome de imagem; sparingly (o script controla o mouse e o teclado do usuário); `wails3 dev` deixa ruído em `frontend/bindings/**` e `go.mod` (reverter só esses caminhos com `git checkout -- <path>` se o conteúdo não mudou de verdade); nunca commitar screenshots, `bin/` ou scratch.
- `wails3 dev` às vezes não conecta no timeout de ~5 s: tentar de novo.
- Textos de UI e mensagens em português.

### Variáveis de ambiente de desenvolvimento/verificação

| Variável | Efeito |
|---|---|
| `HQREADER_DATA_DIR` | Pasta de dados (banco `library.db`). Padrão: `os.UserConfigDir()/hqreader`. A verificação usa uma pasta temporária para não tocar no banco real do usuário. |
| `HQREADER_PICK_FOLDER` | Se definida, o seletor de pasta devolve os caminhos dela em vez de abrir o diálogo nativo (o resto do fluxo "Adicionar biblioteca" é real). Aceita **vários caminhos separados por `;`**: cada chamada consome o próximo, e esgotada a lista repete o último (permite verificar "adicionar de novo" e "adicionar subpasta" numa mesma execução). |
| `HQREADER_SCAN_DELAY_MS` | Atraso artificial por arquivo no scanner, para observar a chegada gradual e abrir livros durante o scan. |
| `HQREADER_BOOK` | Atalho de desenvolvimento: abre direto no leitor esse arquivo, sem biblioteca nem progresso (mantém o script `verify-reader-interactions.ps1` reutilizável como regressão do leitor). |

### Tokens do tema Papel (partida; valores dos mockups aprovados)

| Token | Claro | Escuro |
|---|---|---|
| bg | `#f3ede2` | `#17130f` |
| ink (texto) | `#1d1a16` | `#f3ece0` |
| muted | `#80766a` | `#a39785` |
| accent | `#d9432b` | `#ff6a45` |
| accent2 | `#f0a63c` | `#f0b04a` |
| card | `#fffaf0` | `#211b14` |
| line (bordas 2px) | `#1d1a16` | `#f3ece0` |
| shadow (cartões, 3–4px chapada) | `#1d1a16` | `#000000` |
| cover-shadow | `#1d1a16` | `#ff6a45` |
| skeleton | `#e7dfcf` | `#2b2219` |
| track | `rgba(29,26,22,.15)` | `rgba(243,236,224,.18)` |
| btn-bg / btn-text | `#f0a63c` / `#1d1a16` | `#ff6a45` / `#17130f` |

Tipografia: serifa do sistema (`Georgia, "Times New Roman", serif`) para títulos e identidade; sans do sistema (`system-ui`) para rótulos pequenos. Fonte Geist deixa de ser usada (remover `@fontsource-variable/geist`). Raio ~2–3 px. Gradientes dos placeholders de capa (pares c1→c2): `#c9402b→#f0a63c`, `#1f4e79→#6fa8d6`, `#5a2a6b→#c07bd1`, `#8a6a12→#f2cf5b`, `#1f6b4a→#7bd1a2`, `#7a2e3a→#e08a9a`, `#2f4f4f→#8fbfbf`, `#4a3b8f→#a99bff`.

### Strings de UI (nomes de controles, ASCII)

`Adicionar biblioteca`, `Atualizar tudo`, `Atualizar`, `Cancelar scan`, `Remover biblioteca` (item do menu), `Remover` e `Cancelar` (confirmação), `Voltar para a biblioteca`, `Tema claro`, `Tema escuro`, `Tema do sistema` (botões do seletor, com `aria-pressed`), `Opcoes da biblioteca` (botão "⋯"), `Minhas HQs` (raiz do caminho de navegação e título da tela inicial). Estado vazio: título "Sua estante está vazia". Cartões de livro são botões cujo nome acessível contém o título e o status ("Novo", "34 / 120", "Lido"); livro `missing` mostra "Ausente" no lugar do status; livro `broken` mostra "Arquivo com problema" e o motivo ("protegido por senha", "corrompido", "formato não suportado"); cartões de pasta, o nome da pasta. Aviso de scan interrompido: "scan incompleto". Contagens: "N HQs · M pastas", "N encontrados…". Avisos de adicionar: "já cadastrada" (`exists`), "dentro de uma biblioteca" (`inside_library`), "contém uma biblioteca" (`contains_library`), "pasta não encontrada" (`not_found`).

### Códigos de erro (campo `ErrorCode`, strings estáveis)

`AddResult`: `cancelled`, `not_found`, `exists`, `inside_library`, `contains_library`. `OpenedBook`: `not_found`, `missing`, `password`, `corrupted`, `unsupported`. Vazio = sucesso. O frontend traduz cada código para texto.

## Review Focus

Os cinco modos de falha mais prováveis para quem usa, e a task que fixa cada um com teste:

1. **Pasta com milhares de arquivos:** o scan não pode travar a interface nem inundar o frontend (um evento por arquivo). Teste `TestScanLargeFolderBatchesEvents` (Task 4); renderização com `content-visibility` e checagem numa pasta grande (Tasks 9 e 11).
2. **Arquivo corrompido ou protegido por senha no meio do scan:** vira `broken` com motivo e o scan segue. `TestScanBrokenFileDoesNotStopScan` (Task 4); cartão `broken` na Task 11.
3. **Remover biblioteca durante o scan:** cancela o scan, espera terminar e só então apaga, sem erro de FK nem goroutine escrevendo numa raiz inexistente. `TestRemoveLibraryCancelsRunningScan` (Task 5).
4. **Scan cancelado ou interrompido nunca marca livros como `missing`** e não preenche `scanned_at`. `TestCancelledScanKeepsBooksAndScannedAt` (Task 4).
5. **Caminhos com acentos, espaços, caminho longo, barras/maiúsculas diferentes, e pasta que contém ou está dentro de uma biblioteca existente:** mesma pasta = mesmo id, sem duplicar livros. `TestPathIDNormalization`, `TestScanUnicodeAndLongPaths` (Task 4), `TestAddLibraryOverlapRules` (Task 5).

Também fixados: banco de versão futura recusado sem alterar o arquivo (`TestOpenRefusesNewerSchema`, Task 3); leitura concorrente com o scan escrevendo, sem `SQLITE_BUSY` (`TestConcurrentReadDuringWrite`, Task 3); progresso salvo ao fechar o livro mesmo antes do debounce (`TestProgressFlushedOnCloseBook`, Task 5).

## Decisões deste plano (revise)

1. **`Inspect` em vez de `Open` no scan:** CBZ usa o índice do zip; CBR lê só cabeçalhos e guarda só o `ComicInfo.xml` em memória, sem pasta temporária. `ComicInfo.xml` malformado não torna o livro `broken` (título cai para o nome do arquivo).
2. **Testes do `LibraryService` usam o store real** num arquivo temporário em vez de um store mockado (a spec §7 dizia "store mockado"): `modernc` é Go puro e barato, e um mock de SQL testaria o mock (upsert, `ON CONFLICT`, cascata).
3. **"Scan incompleto"** = `libraries.scanned_at` nulo e nenhum scan rodando; um rescan cancelado de uma biblioteca já escaneada mantém o `scanned_at` anterior.
4. **Marcação de `missing` por token de scan** (`books.last_seen` recebe o instante de início do scan em cada arquivo visto; no fim de um scan **concluído**, `missing = last_seen < token`), sem lista gigante de IDs.
5. **Contagens nos cartões de pasta** = livros não-`missing` da subárvore e pastas filhas diretas; durante o scan o frontend funde os lotes de livros recebidos e refaz a consulta da pasta atual no máximo a cada 1 s e ao concluir (os eventos de lote não carregam contagens).
6. **Progresso:** o frontend reporta a visão assentada do leitor (primeiro e último índice da visão) via `SaveProgress`; o backend faz debounce (padrão 750 ms por livro), grava `furthest_page = max(...)`, e descarrega no `CloseBook` e no `ServiceShutdown`. Abrir um livro já conta como "começado" (status deixa de ser "Novo"). O gancho em `reader.Service` que a spec §7 previa **não** é necessário; `internal/reader` fica sem mudança.
7. **`HQREADER_BOOK` sobrevive** como atalho de desenvolvimento (tabela acima), para reaproveitar `verify-reader-interactions.ps1` como regressão.
8. **Diálogo de confirmação próprio e mínimo** (`role="dialog"`), sem adicionar o `Dialog` do shadcn.
9. **Id do caminho:** `NormalizePath` = `filepath.Abs` + `filepath.Clean`, tira separador final; no Windows também troca `\` por `/` e passa para minúsculas. `PathID` = primeiros 32 hex de `sha256(NormalizePath(p))`. O `path` gravado é o limpo, com a caixa original.

---

### Task 1: Spike — API real do Wails, desempenho do modernc, custo de CBR

Padrão da Etapa 0: código descartável, resultado escrito. As Tasks 6, 8 e 9 usam a API **confirmada** aqui; se divergir do que o plano assume (nomes abaixo, lidos do código-fonte do módulo), vale o achado e o implementador de cada task adapta e registra no relatório.

**Files:**
- Create: `spikes/06-etapa4-apis/` (módulo Go próprio, copiado de `spikes/wails-poc`, mais um benchmark SQLite)
- Create: `spikes/06-etapa4-apis-finding.md`

**Interfaces:**
- Produces (no arquivo de achados, com trechos de código exatos que funcionaram): (a) como um método de serviço abre o diálogo nativo de pasta e o que ele devolve ao cancelar (esperado: `application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{CanChooseDirectories:true, CanChooseFiles:false, Title:...}).PromptForSingleSelection()`); (b) como um serviço emite eventos a partir de goroutines (esperado: `application.Get().Event.Emit(name, payload)`), se é seguro concorrentemente, e o formato do payload no JS (esperado: `import { Events } from "@wailsio/runtime"; Events.On(name, e => e.data)`; caixa dos campos sem tags json); (c) como os bindings tipam structs aninhados, slices, `int64`, `bool`, ponteiro nulo e quais comandos regeneram os bindings; (d) `ServiceStartup`/`ServiceShutdown` rodam e quando; (e) DSN do `modernc` que liga WAL + `foreign_keys` + `busy_timeout`; (f) números de desempenho; (g) custo real de CBR; (h) decisão final de `NormalizePath`.

- [ ] **Step 1: Copiar o scaffold e montar o spike Wails.** Copiar `spikes/wails-poc` para `spikes/06-etapa4-apis`, renomear o módulo, remover os serviços de exemplo e criar um serviço com: `PickFolder() (string, error)`, `EmitBurst(n int)` (goroutines emitindo eventos com payload struct), `Sample() SampleResult` (struct aninhada com slice de structs, `int64`, `bool`, string) e `Slow(ms int)`; frontend vanilla mínimo com botões e área de log que assina o evento.
- [ ] **Step 2: Rodar com `wails3 dev` e provar cada item (a)–(d) de verdade.** Para o diálogo: clicar o botão com mouse real, enviar `{ESC}` por `SendKeys` depois de ~1 s e registrar exatamente o que `PromptForSingleSelection` devolve (string vazia e erro nulo? erro?). Para eventos: contar quantos chegam de um burst de 1000 emitidos de 8 goroutines e se algum se perde ou chega fora de ordem. Registrar nos achados os bindings gerados (arquivo e assinatura TS de cada método) e o comando que os regenera sem abrir janela, se existir.
- [ ] **Step 3: Medir o `modernc`.** Programa Go no mesmo módulo: abrir o banco com a DSN candidata, inserir 5 000 linhas do formato de `books` em transações de lote (100/500/1000) e medir tempo; medir leitura concorrente (4 leitores em loop) enquanto um escritor insere em lotes e contar erros `database is locked`/`SQLITE_BUSY`; comparar `SetMaxOpenConns(1)` com conexões livres. Confirmar `CGO_ENABLED=0 go build`. Registrar tamanho do binário final do spike.
- [ ] **Step 4: Medir o CBR.** Com uma amostra real (`HQREADER_TEST_CBR`, ou `C:\Users\mfdin\Downloads\Os Fabulosos X-Men.v1 - 175 de 544.cbr` se existir, ou `spikes/rar-cost/testdata/sample-solid.cbr`), cronometrar `rardecode` só percorrendo cabeçalhos com `Next()` sem ler corpo, e guardando em memória apenas `ComicInfo.xml`, contra o `archive.Open` atual (extração completa). Registrar se a amostra é sólida (`hdr.Solid`) e quanto custa cada abordagem. Sem amostra, registrar isso e medir só o caminho CBZ.
- [ ] **Step 5: Decidir `NormalizePath`.** Com um teste rápido no Windows, mostrar que `C:\Comics\X`, `c:/comics/x/` e `C:\Comics\.\X` colapsam para o mesmo id e `C:\Comics\Y` não; testar uma pasta de 300+ caracteres sob `t.TempDir()` (Go abre caminhos longos sozinho?) e acentos/espaços. Registrar o resultado e qualquer ajuste à Decisão 9.
- [ ] **Step 6: Escrever `spikes/06-etapa4-apis-finding.md`** com veredito GO/GO COM AJUSTE por item (a)–(h), os trechos de código confirmados e os números. Um achado que contradiga a spec ou o plano é destacado no topo.
- [ ] **Step 7: Commit** (`git add spikes/06-etapa4-apis spikes/06-etapa4-apis-finding.md`; mensagem `spike: valida API do Wails (dialogo, eventos, bindings), modernc e custo de CBR para a Etapa 4`; sem trailer). Não commitar `node_modules`, `bin/`, `dist` nem o banco do benchmark.

---

### Task 2: `archive.Inspect` — inspeção leve

**Files:**
- Create: `internal/archive/inspect.go`, `internal/archive/inspect_test.go`
- Modify: `internal/archive/rar.go` (só para extrair o que for compartilhável, sem mudar o comportamento de `newRarArchive`)

**Interfaces:**
- Consumes: `buildPageList`, `rawEntry`, `parseComicInfo`, `classifyRarError`, sentinelas `ErrCorrupted`/`ErrPasswordProtected`/`ErrUnsupportedFormat`, `tempDirRoot` (variável de teste).
- Produces: `type Info struct { Format string; PageCount int; ComicInfo *ComicInfo }` (`Format` = `"cbz"` ou `"cbr"`, por extensão: `.cbz`/`.zip` → cbz, `.cbr`/`.rar` → cbr) e `func Inspect(path string) (Info, error)`. Erros: os mesmos sentinelas; arquivo inexistente/sem permissão devolve erro que satisfaz `errors.Is(err, fs.ErrNotExist)` / `fs.ErrPermission`.

- [ ] **Step 1: Testes (devem falhar).** Em `inspect_test.go`, table-driven com CBZ gerados em `t.TempDir()`:
  - `TestInspectZip`: zip com 3 imagens + `Thumbs.db` + `ComicInfo.xml` (Title "Edição Especial", Series "S", Number "7") → `PageCount == 3`, `Format == "cbz"`, `ComicInfo.Title == "Edição Especial"`, `Series == "S"`, `Number == "7"`.
  - `TestInspectZipWithoutComicInfo`: `ComicInfo == nil`, sem erro.
  - `TestInspectZipMalformedComicInfo`: XML quebrado → sem erro, `ComicInfo == nil`, `PageCount` correto.
  - `TestInspectZipCorrupted`: arquivo de bytes aleatórios `.cbz` → `errors.Is(err, ErrCorrupted)`.
  - `TestInspectUnsupportedExtension`: `.pdf` → `ErrUnsupportedFormat`.
  - `TestInspectMissingFile`: caminho inexistente → `errors.Is(err, fs.ErrNotExist)`.
  - `TestInspectRarCorruptedSynthetic`: assinatura RAR5 + lixo (como `TestRarArchiveCorruptedSynthetic`) → `ErrCorrupted`.
  - `TestInspectRarRealSample` (pula sem amostra, via `realCBRPath`): `PageCount` igual ao de `newRarArchive(path).Pages()`, e **nenhuma pasta temporária criada** (apontar `tempDirRoot` para um dir vazio do teste e afirmar que continua vazio depois); `ComicInfo` igual ao de `Metadata()`.
- [ ] **Step 2: Rodar** `go test ./internal/archive -run Inspect -count=1` → falha (símbolos inexistentes).
- [ ] **Step 3: Implementar `Inspect`.** Dispatch por extensão como `Open`. ZIP: pode reutilizar `newZipArchive` + `Pages` + `Metadata` + `Close` (o diretório central é barato). RAR: laço de `rardecode` por cabeçalhos; para cada entrada que não é diretório, aplicar `sanitizeEntryName` e acumular `rawEntry{Name, Size: hdr.UnPackedSize}` sem gravar nada em disco; só quando `strings.EqualFold(hdr.Name, "ComicInfo.xml")` ler o corpo para memória (limite 1 MiB) e `parseComicInfo`; erros via `classifyRarError`; `PageCount = len(buildPageList(entries))`. `ComicInfo` malformado → nil, sem erro.
- [ ] **Step 4: Rodar** `go test ./internal/archive -count=1` → tudo passa (os pulados continuam pulados).
- [ ] **Step 5: Commit** (`git add internal/archive`; mensagem `archive: Inspect leve (sem extrair CBR) para o scanner da biblioteca`).

---

### Task 3: `internal/store` — SQLite, migrações, consultas

**Files:**
- Create: `internal/store/store.go`, `migrations.go`, `libraries.go`, `books.go`, `progress.go`, `store_test.go`, `libraries_test.go`, `books_test.go`, `progress_test.go`
- Modify: `go.mod`, `go.sum` (`modernc.org/sqlite`, versão estável atual, fixada; se o piso do `go` subir por exigência do módulo, é aceitável como no Wails)

**Interfaces:**
- Consumes: nada de tasks anteriores (usa a DSN confirmada na Task 1).
- Produces (pacote `store`):
  - Tipos: `Library{ID, Path, Name, ParentID, RootID string; AddedAt, ScannedAt int64}` (`ParentID == ""` = raiz; `ScannedAt == 0` = nunca concluído); `Book{ID, LibraryID, Path, Format, Title, Series, Number, BrokenReason string; Size, MTime, AddedAt, LastSeen int64; PageCount int; Missing, Broken bool}`; `Progress{BookID string; CurrentPage, FurthestPage int; UpdatedAt int64}`; `BookWithProgress{Book; Progress *Progress}`; `Fingerprint{Size, MTime int64}`; `Stats{Books, Folders int}`; `var ErrSchemaTooNew`, `var ErrNotFound`; `const SchemaVersion = 1`.
  - `func Open(path string) (*Store, error)` (cria o arquivo, aplica pragmas WAL/`foreign_keys`/`busy_timeout`, migra), `func (s *Store) Close() error`.
  - Bibliotecas: `UpsertLibraries(ctx, libs []Library) error` (pais antes dos filhos; atualiza nome/caminho/pai/raiz, **não** sobrescreve `added_at` nem `scanned_at`), `GetLibrary(ctx, id) (Library, bool, error)`, `ListRoots(ctx) ([]Library, error)` (ordem natural por nome, sem diferenciar maiúsculas), `ListChildren(ctx, parentID string) ([]Library, error)` (idem), `Ancestors(ctx, id string) ([]Library, error)` (raiz primeiro, o próprio por último), `DeleteRoot(ctx, rootID string) error` (cascata: sub-bibliotecas, livros, progresso), `MarkScanned(ctx, rootID string, at int64) error`, `Stats(ctx, libraryID string) (Stats, error)` (livros não-`missing` da subárvore; pastas filhas diretas).
  - Livros: `Fingerprints(ctx, rootID string) (map[string]Fingerprint, error)`, `UpsertBooks(ctx, books []Book) error` (uma transação; grava `last_seen`, zera `missing`), `TouchBooks(ctx, ids []string, seen int64) error` (atualiza `last_seen`, zera `missing`), `MarkMissing(ctx, rootID string, token int64) (int, error)` (`missing = 1` onde `last_seen < token`, só na raiz dada; devolve quantos), `ListBooks(ctx, libraryID string) ([]BookWithProgress, error)` (ordem natural pelo nome do arquivo), `GetBook(ctx, id string) (BookWithProgress, bool, error)`.
  - Progresso: `SaveProgress(ctx, bookID string, current, furthest int, at int64) error` (`furthest_page = max(existente, furthest)`; livro inexistente → `ErrNotFound`).

- [ ] **Step 1: Testes (devem falhar).** Todos com `Open(filepath.Join(t.TempDir(), "t.db"))`:
  - `TestOpenCreatesSchemaV1`: `PRAGMA user_version == 1`, `journal_mode == "wal"`, `foreign_keys == 1`; reabrir não refaz a migração.
  - `TestOpenRefusesNewerSchema`: criar o banco, `PRAGMA user_version = 99`, fechar; `Open` → `errors.Is(err, ErrSchemaTooNew)` e o arquivo continua byte-idêntico (hash antes/depois).
  - `TestLibraryTree`: raiz + 2 filhas + 1 neta; `ListChildren`, `Ancestors(neta)` == [raiz, filha, neta], `ListRoots` só a raiz; reupsert muda o nome sem mudar `AddedAt`.
  - `TestDeleteRootCascades`: raiz com sub-bibliotecas, livros e progresso; `DeleteRoot` → nenhuma linha restante em `libraries`/`books`/`reading_progress` daquela raiz, e uma segunda raiz intacta.
  - `TestStatsCountsSubtreeNonMissing`: 3 livros na raiz, 2 na filha (1 `missing`), 1 na neta → `Stats(raiz) == {Books: 5, Folders: 1}`, `Stats(filha) == {Books: 2, Folders: 1}`.
  - `TestUpsertBooksAndFingerprints`: upsert de 3 livros; `Fingerprints` devolve tamanho/mtime; reupsert do mesmo `ID` atualiza campos e não duplica.
  - `TestMarkMissing`: A (visto no scan token 100), B (`last_seen` 50) → `MarkMissing(raiz, 100)` marca só B, devolve 1; `TouchBooks([B], 120)` e `MarkMissing(raiz, 120)` desmarcam B; livros de **outra** raiz não são tocados.
  - `TestSaveProgressFurthestIsMonotonic`: `SaveProgress(b, 5, 8)` depois `(b, 2, 3)` → `current 2`, `furthest 8`; `SaveProgress("inexistente", ...)` → `ErrNotFound`.
  - `TestListBooksNaturalOrderWithProgress`: arquivos `HQ 2.cbz`, `HQ 10.cbz`, `HQ 1.cbz` → ordem 1, 2, 10; o que tem progresso vem com `Progress != nil`.
  - `TestConcurrentReadDuringWrite`: um escritor insere 2 000 livros em lotes de 100 enquanto 4 goroutines chamam `ListBooks`/`Stats` em loop até o fim; nenhum erro (nem `database is locked`).
- [ ] **Step 2: Rodar** `go test ./internal/store -count=1` → falha.
- [ ] **Step 3: Implementar.** `migrations.go`: slice de scripts por versão aplicados em transação, com `PRAGMA user_version`; migração 1 cria `libraries`, `books` (com `library_id`, `last_seen`, `broken_reason`, índices em `books(library_id)`, `libraries(parent_id)`, `libraries(root_id)`) e `reading_progress` (com `furthest_page`) conforme a spec §7 e o spec mestre §3.5; chaves estrangeiras `ON DELETE CASCADE` (raiz tem `root_id` = o próprio `id`). Ordem natural no SQL: ordenar em Go (reaproveitar uma função de ordem natural simples) em vez de `COLLATE` customizado, ou registrar collation; escolha livre desde que o teste de ordem passe. Estatísticas por CTE recursivo.
- [ ] **Step 4: Rodar** `go test ./internal/store -count=1` → passa; `CGO_ENABLED=0 go vet ./internal/store`.
- [ ] **Step 5: Commit** (`git add internal/store go.mod go.sum`; mensagem `store: SQLite (modernc) com migracoes, bibliotecas, livros e progresso`).

---

### Task 4: `internal/library` — identidade de caminho e scanner

**Files:**
- Create: `internal/library/identity.go`, `scanner.go`, `walk.go`, `events.go`, `identity_test.go`, `scanner_test.go`, `testutil_test.go`

**Interfaces:**
- Consumes: `store.*` (Task 3), `archive.Info`/`archive.Inspect`/sentinelas (Task 2).
- Produces:
  - `func NormalizePath(path string) string`, `func PathID(path string) string`, `func IsWithin(child, parent string) bool` (limite de separador: `C:\a\bc` **não** está dentro de `C:\a\b`; igual a si mesmo não conta como "dentro").
  - `type InspectFunc func(path string) (archive.Info, error)`.
  - `type Options struct{ Workers, BatchSize int; FlushEvery, PerFileDelay time.Duration; Now func() time.Time }` (padrões: workers `min(4, NumCPU)`, lote 50, flush 100 ms).
  - `type ScanEvent struct{ RootID, Kind string; Found, Validated int; Folders []FolderItem; Books []BookItem; Message string }` com `Kind ∈ {"started","walked","batch","done","cancelled","error"}`; `FolderItem`/`BookItem` definidos em `events.go` exatamente como na Task 5 (ver abaixo; esta task os declara, a Task 5 os usa).
  - `type FolderItem struct{ ID, ParentID, Name string; BookCount, FolderCount int }`; `type BookItem struct{ ID, LibraryID, Title, Subtitle, FileName, Format, Status, BrokenReason string; PageCount, CurrentPage, FurthestPage int; Missing, Broken bool }` (`Status ∈ {"new","reading","read"}`; título = `ComicInfo.Title` ou nome do arquivo sem extensão; `Subtitle` = `Series #Number` se houver); `func ToBookItem(b store.BookWithProgress) BookItem`.
  - `func NewScanner(st *store.Store, inspect InspectFunc, opts Options) *Scanner`; `func (s *Scanner) Scan(ctx context.Context, root store.Library, emit func(ScanEvent)) error` (devolve `nil` ao concluir e `ctx.Err()` ao cancelar, depois de emitir `"cancelled"`).

- [ ] **Step 1: Testes de identidade (devem falhar).** `TestPathIDNormalization` (table; no Windows): `C:\Comics\X`, `c:/comics/x/`, `C:\Comics\.\X`, `C:\COMICS\x` → mesmo `PathID`; `C:\Comics\Y` e `C:\Comics\X\Y` diferem; `PathID` tem 32 hex. `TestIsWithin` (table): filho direto, neto, prefixo falso (`C:\a\bc` vs `C:\a\b`), igual a si mesmo (`false`), pai dentro do filho (`false`), barras mistas.
- [ ] **Step 2: Testes do scanner (devem falhar).** Helpers em `testutil_test.go`: criar CBZ mínimo (N páginas PNG minúsculas, `ComicInfo` opcional), criar árvores em `t.TempDir()`, um `InspectFunc` falso com contagem de chamadas e falhas programadas, e um coletor de eventos protegido por mutex. Testes:
  - `TestScanBuildsLibraryTree`: árvore `raiz/A/x.cbz`, `raiz/A/B/y.cbz`, `raiz/C/` (vazia) e `raiz/D/leia-me.txt` → no banco: raiz, `A`, `A/B`; **não** `C` nem `D`; livros com `library_id` da pasta certa; `.cbz`, `.CBR` (maiúscula) e `.zip`? só `.cbz`/`.cbr` entram (spec §5.2): um `.zip` na árvore é ignorado.
  - `TestScanSkipsUnchangedFiles`: segundo scan sem mudanças → o `InspectFunc` falso é chamado 0 vezes; mudar `size`/`mtime` de um arquivo → chamado só para ele.
  - `TestScanBrokenFileDoesNotStopScan`: 5 arquivos, o 3º com `ErrCorrupted` e o 4º com `ErrPasswordProtected` (falso) → 5 livros gravados, 2 `Broken` com `BrokenReason` `"corrupted"`/`"password"`, os outros normais, evento `done`.
  - `TestScanRescanMarksMissingAndRecovers`: scan; apagar 1 arquivo; novo scan → esse livro `Missing`, os demais não; recriar o arquivo; novo scan → volta a `Missing == false`.
  - `TestCancelledScanKeepsBooksAndScannedAt`: `InspectFunc` falso que bloqueia no 5º arquivo até o teste cancelar o `ctx`; `Scan` devolve `context.Canceled`; **nenhum** livro anterior vira `Missing` (inclusive em rescan de raiz que já tinha livros), `ScannedAt` da raiz continua o valor anterior (0 num primeiro scan), evento `"cancelled"` emitido. Rodar 20 vezes (`-count=20`) para flagrar corrida.
  - `TestScanLargeFolderBatchesEvents`: 2 000 CBZ de uma página numa pasta (rápido) → 2 000 livros; número de eventos `"batch"` ≤ 2 000/BatchSize + 20; soma dos livros nos lotes = 2 000; `Found == Validated == 2000` no `done`.
  - `TestScanUnicodeAndLongPaths`: pasta `Quadrinhos Épicos ção/Série Ñ` com espaços e acentos, e um caminho aninhado com mais de 300 caracteres no total → livros achados e id estável (re-scan não duplica).
  - `TestScanReportsProgressPhases`: eventos em ordem `started` → `walked` (com `Found` final e as pastas) → `batch`* → `done`; `Validated` não decresce.
  - `TestScanPerFileDelayHonorsCancel`: `PerFileDelay` 50 ms, cancelar após 120 ms → retorna em < 500 ms.
- [ ] **Step 3: Rodar** `go test ./internal/library -count=1` → falha.
- [ ] **Step 4: Implementar.** Fase 1 (`walk.go`, `filepath.WalkDir`, sem abrir arquivos): coleta caminhos `.cbz`/`.cbr` (extensão sem diferenciar caixa) e marca todas as pastas ancestrais, até a raiz, como sub-bibliotecas; grava as pastas (pais antes dos filhos, `Name = filepath.Base`) e emite `walked`; diretório ilegível é registrado e pulado (`Message` em evento `error` não fatal). Fase 2: lê `Fingerprints(rootID)` uma vez; arquivo com mesmo `size`+`mtime` só entra em `TouchBooks` (em lote); os demais vão para um pool de `Workers` que chama `InspectFunc`; resultado vira `store.Book` (`Broken`/`BrokenReason` ∈ `corrupted|password|unsupported|unreadable`); gravação e emissão em lotes (`BatchSize` ou `FlushEvery`, o que vier primeiro); `PerFileDelay` respeitando `ctx`. Fim: se **não** cancelado, `MarkMissing(root, token)` e `MarkScanned`; `token` = instante de início do scan. Cancelado: nada de `MarkMissing`/`MarkScanned`. Nada marca `missing` se a fase 1 não terminou.
- [ ] **Step 5: Rodar** `go test ./internal/library -count=1`, e `go test ./internal/library -run TestCancelled -count=20` → passa.
- [ ] **Step 6: Commit** (`git add internal/library`; mensagem `library: identidade de caminho e scanner de duas fases com eventos em lote`).

---

### Task 5: `LibraryService` — bibliotecas, abrir livro, progresso, desligamento

**Files:**
- Create: `internal/library/service.go`, `dto.go`, `progress.go`, `service_test.go`, `progress_test.go`

**Interfaces:**
- Consumes: `store.*`, `Scanner`/`ScanEvent`/`FolderItem`/`BookItem` (Task 4), `reader.Service` (`Open(path) (OpenResult{BookID, PageCount}, error)`, `SetCurrentPage`, `Close`), `archive` sentinelas.
- Produces (todos métodos exportados são ligados ao frontend; sem tags json, campos em PascalCase como o restante do projeto):
  - `type Picker func() (string, error)` (`""` = cancelado); `type EmitFunc func(name string, payload any)`; `func SequencePicker(paths []string, fallback Picker) Picker` (devolve os caminhos em ordem, um por chamada, e depois repete o último; lista vazia delega a `fallback`; seguro para chamadas concorrentes) — usado pelo `main` para `HQREADER_PICK_FOLDER`.
  - `type Config struct{ Store *store.Store; Reader *reader.Service; Scanner *Scanner; Pick Picker; Emit EmitFunc; ProgressDebounce time.Duration; DevBook string }`; `func NewService(cfg Config) *Service`.
  - DTOs: `RootSummary{ID, Name, Path string; BookCount, FolderCount int; ScannedAt int64; Scanning bool; Found, Validated int}`; `Crumb{ID, Name string}`; `FolderView{ID, Name, RootID string; Crumbs []Crumb; Folders []FolderItem; Books []BookItem; Scanning bool}` (`Crumbs` não inclui "Minhas HQs", que é do frontend); `AddResult{RootID, ErrorCode, Message string}`; `OpenedBook{ErrorCode, Session, LibraryBookID, Title string; PageCount, StartPage, FurthestPage int}`.
  - Métodos: `AddLibrary() (AddResult, error)`, `ListRoots() ([]RootSummary, error)`, `ListFolder(libraryID string) (FolderView, error)`, `RemoveLibrary(rootID string) error`, `StartScan(rootID string) error`, `StartScanAll() error`, `CancelScan(rootID string) error`, `OpenBook(libraryBookID string) (OpenedBook, error)`, `OpenFile(path string) (OpenedBook, error)` (atalho de dev; `LibraryBookID == ""`, sem progresso), `CloseBook(session, libraryBookID string) error`, `SaveProgress(libraryBookID string, current, furthest int) error`, `DevStartupBook() string`, `ServiceShutdown() error`.
  - Evento emitido: nome `library:scan`, payload `ScanEvent` (Task 4).

- [ ] **Step 1: Testes (devem falhar).** Banco temporário real, `reader.NewService()` real, `Picker` e `Emit` falsos, scanner com `InspectFunc` real sobre CBZ gerados:
  - `TestAddLibraryOverlapRules` (table): pasta nova → `RootID` preenchido e scan iniciado; pasta já cadastrada (também com barras/caixa diferentes) → `ErrorCode "exists"` e `RootID` da existente; pasta **dentro** de biblioteca existente → `"inside_library"`; pasta que **contém** uma biblioteca existente → `"contains_library"`; caminho inexistente → `"not_found"`; picker devolvendo `""` → `"cancelled"`; prefixo falso (`...\bc` vs `...\b`) não conta como sobreposição.
  - `TestListFolderHierarchyAndCrumbs`: após scan de `raiz/A/B`, `ListFolder(B)` tem `Crumbs == [raiz, A, B]` (nomes), `Books` só os de B; `ListFolder(raiz)` tem `Folders == [A]` com `BookCount` da subárvore e `FolderCount` 1.
  - `TestRemoveLibraryCancelsRunningScan`: scan lento (`PerFileDelay`) em andamento → `RemoveLibrary` retorna sem erro, não há goroutine do scan viva depois (esperar o término com timeout 2 s), nenhuma linha restante da raiz, e **nenhum** evento `batch` chega depois do retorno de `RemoveLibrary`. Outra raiz intacta.
  - `TestStartScanIsIdempotentWhileRunning`: segundo `StartScan` da mesma raiz durante o scan não inicia outro (contar chamadas do `InspectFunc` por arquivo: cada arquivo inspecionado uma vez); `ListRoots` mostra `Scanning == true` e contadores durante o scan.
  - `TestOpenBook`: livro bom → `ErrorCode == ""`, `Session` não vazio, `PageCount` igual ao do arquivo, `StartPage == 0`, o `reader.Service` reconhece a sessão (`PageInfo(session, 0)` funciona); livro sem progresso → `StartPage 0`; com progresso `current 7` → `StartPage 7`; progresso além do fim (`current` ≥ `PageCount`) → `StartPage == PageCount-1`; arquivo apagado do disco → `ErrorCode "missing"` e o livro vira `Missing` no banco; CBZ corrompido → `"corrupted"`; livro já `Broken` → mesmo código sem reabrir; id inexistente → `"not_found"`.
  - `TestStatusFromProgress` (table): sem progresso → `"new"`; `current 3, furthest 3` → `"reading"`; `furthest == PageCount-1` → `"read"` mesmo com `current 0`; livro de 1 página aberto (`furthest 0`) → `"read"`.
  - `TestProgressDebounceCoalescesWrites` (debounce 20 ms): 50 chamadas rápidas a `SaveProgress` → no máximo 2 gravações no banco (contar via `Fingerprint`/`UpdatedAt` ou um wrapper), valor final correto.
  - `TestProgressFlushedOnCloseBook`: debounce longo (10 s); `SaveProgress(b, 4, 6)` → `CloseBook` → o banco já tem `current 4`, `furthest 6`. `TestProgressFlushedOnShutdown`: idem com `ServiceShutdown`. `furthest` não regride em `SaveProgress(b, 1, 2)` depois.
  - `TestServiceShutdownCancelsScansAndDoesNotMarkMissing`: scan lento em andamento, `ServiceShutdown` → retorna em < 2 s, livros anteriores não ficam `Missing`.
  - `TestScanEventsEmittedOnLibraryScanChannel`: o `EmitFunc` recebe `name == "library:scan"` e payloads `ScanEvent` com `RootID` da raiz.
  - `TestSequencePicker` (table): `["a","b"]` → `a`, `b`, `b`, `b`; lista vazia com `fallback` que devolve `"x"` → `x`; chamadas concorrentes (20 goroutines) não repetem nem pulam além da regra "um por chamada, depois o último".
- [ ] **Step 2: Rodar** `go test ./internal/library -count=1` → falha.
- [ ] **Step 3: Implementar.** Gerenciador de scans: mapa `rootID → {cancel, done chan}` sob mutex; `StartScan` ignora se já existe; goroutine do scan chama `Scanner.Scan` com `emit` que repassa a `Config.Emit("library:scan", ev)`; `RemoveLibrary` cancela, espera `done`, só então `DeleteRoot`. `AddLibrary`: `Pick`, normalizar, regras de sobreposição por `IsWithin` contra todas as raízes, `UpsertLibraries` da raiz (`RootID == ID`), `StartScan`. `OpenBook`: consulta o livro, mapeia erros de `archive` para `ErrorCode`, `reader.Service.Open`, `SetCurrentPage(session, StartPage)`. Progresso: mapa por livro com `time.AfterFunc` (debounce de `ProgressDebounce`, padrão 750 ms), último valor vence com `furthest` máximo; `CloseBook` fecha a sessão do `reader.Service`, descarrega o progresso daquele livro; `ServiceShutdown` cancela todos os scans (e espera), descarrega tudo e **não** fecha o `*store.Store` (quem abre fecha, `main`). `OpenFile`: só `reader.Service.Open`. Conversão para `BookItem`/`RootSummary` reaproveita `ToBookItem` e `Stats`.
- [ ] **Step 4: Rodar** `go test ./internal/... -count=1` e `go test ./internal/library -run 'Remove|Shutdown|Debounce' -count=10`.
- [ ] **Step 5: Commit** (`git add internal/library`; mensagem `library: LibraryService (bibliotecas, abrir livro, progresso com debounce, desligamento)`).

---

### Task 6: Shell Wails — ligação, diálogo, eventos, bindings

**Files:**
- Modify: `cmd/hqreader/main.go`
- Create: `cmd/hqreader/wiring.go` (adaptadores finos para `Picker` e `EmitFunc`; sem testes, são cola de `application`)
- Modify (gerados): `cmd/hqreader/frontend/bindings/**`
- Modify: `cmd/hqreader/README.md` (tabela das variáveis de ambiente de desenvolvimento)

**Interfaces:**
- Consumes: `library.NewService`, `library.Config`, `store.Open`, `library.NewScanner`, `archive.Inspect`, `reader.NewService`, achados da Task 1 (assinaturas reais do Wails).
- Produces: app que registra `reader.Service` e `library.Service` como serviços Wails, banco em `<dados>/library.db`, picker nativo (ou `HQREADER_PICK_FOLDER`), emissor de eventos, `HQREADER_SCAN_DELAY_MS` → `Options.PerFileDelay`, `HQREADER_BOOK` → `Config.DevBook`, `HQREADER_DATA_DIR`; bindings TS de `internal/library` (métodos de `Service` e os DTOs em `models.ts`).

- [ ] **Step 1: Escrever a ligação** conforme os achados da Task 1: `main` cria a pasta de dados (`HQREADER_DATA_DIR` ou `os.UserConfigDir()/hqreader`), `store.Open`, `reader.NewService`, scanner com `archive.Inspect`, picker (`library.SequencePicker(split(HQREADER_PICK_FOLDER, ";"), nativo)`, onde `nativo` é o diálogo nativo de pasta tratando "cancelado" como `""`), emissor (`application.Get().Event.Emit`, resolvido de forma preguiçosa), serviços registrados, `defer store.Close()` depois de `app.Run` retornar. Remover o `log.Fatal` por falta de `HQREADER_BOOK`. Manter `archive.CleanOrphanedTempDirs()` no início.
- [ ] **Step 2: Verificar a compilação** `go build ./...` e `go vet ./cmd/hqreader` (com `frontend/dist` existente) e `go test ./internal/... -count=1`.
- [ ] **Step 3: Regenerar os bindings** pelo comando confirmado na Task 1 (ou rodando `wails3 dev` uma vez) e conferir em `bindings/.../internal/library/service.ts` a presença de todos os métodos da Task 5 com os tipos esperados, e em `models.ts` os DTOs. Reverter apenas ruído (`go.mod` CRLF etc.).
- [ ] **Step 4: Fumaça real.** `wails3 dev` com `HQREADER_DATA_DIR` temporário e `HQREADER_PICK_FOLDER` apontando para uma pasta de CBZ; do console do WebView (ou de um botão temporário **não commitado**) confirmar que `ListRoots` responde vazio e que `AddLibrary` cadastra e emite eventos `library:scan` que o frontend recebe. Registrar no relatório o que foi visto. Parar só o PID iniciado.
- [ ] **Step 5: Documentar** as variáveis de ambiente em `cmd/hqreader/README.md`.
- [ ] **Step 6: Commit** (`git add cmd/hqreader`; mensagem `hqreader: liga biblioteca, banco, dialogo de pasta e eventos no shell Wails`).

---

### Task 7: Frontend — tokens do Papel, tema, barra superior, troca de visões

**Files:**
- Modify: `cmd/hqreader/frontend/src/index.css`, `index.html`, `src/App.tsx`, `src/components/Reader.tsx`, `src/components/ui/button.tsx`, `package.json`, `package-lock.json`
- Create: `src/store/themeStore.ts`, `src/store/viewStore.ts`, `src/components/TopBar.tsx`, `src/components/ThemeToggle.tsx`, `src/components/paper/` (primitivas: `SectionLabel.tsx`, `PaperCard.tsx`)

**Interfaces:**
- Consumes: bindings da Task 6 (`LibraryService.DevStartupBook`, `OpenFile`).
- Produces: `useThemeStore` com `{ mode: "light"|"dark"|"system"; resolved: "light"|"dark"; setMode(m) }` (persistência em `localStorage["hqreader.theme"]` dentro de `try/catch`; aplica/remove a classe `dark` em `<html>`; reage a `prefers-color-scheme` no modo `system`; padrão `system`; um script inline no `<head>` aplica a classe antes da primeira pintura para não piscar); `useViewStore` com `{ view: "library"|"reader"; enterReader(): void; exitReader(): void }`; `TopBar` (logo "hqreader" que volta à tela inicial, seletor de tema com os três botões da tabela de strings; no leitor, botão `Voltar para a biblioteca` e o título do livro); tokens CSS (variáveis `--paper-*` + mapeamento das variáveis do shadcn: `--background`, `--foreground`, `--primary`, `--secondary`, `--card`, `--border`, `--ring`, `--muted`... para o Papel claro e escuro conforme a tabela) em `index.css`, classe `dark` para o escuro.

- [ ] **Step 1: Tokens e tipografia.** Reescrever o bloco de variáveis de `index.css` com a tabela do Papel (claro em `:root`, escuro em `.dark`), serifa para `--font-heading`, remover o import e a dependência `@fontsource-variable/geist`; ajustar `--radius` para ~0.2rem. Remover o `bg-neutral-900` forçado em `html`.
- [ ] **Step 2: Restilizar `button.tsx`** (arquivo do shadcn é fonte do projeto): `default` = preenchido com `--paper-btn-bg`/`--paper-btn-text`, borda 2px `--paper-line`, sombra chapada pequena; `secondary` = cartão com borda 2px; `ghost`; foco visível; `disabled` com opacidade. Mantém a API `variant`/`size`.
- [ ] **Step 3: Tema.** `themeStore` + `ThemeToggle` (três botões, `aria-pressed`, nomes da tabela) + script anti-flash em `index.html`.
- [ ] **Step 4: Visões.** `viewStore`; `App.tsx` mostra `TopBar` e, conforme `view`, a tela da biblioteca (por ora um marcador de posição simples) ou o `Reader`; ao montar, se `DevStartupBook()` devolver caminho, chama `OpenFile(caminho)` e `enterReader()`; o `Reader` segue usando o `loadCurrentBook`/`Current()` atual, que enxerga a sessão aberta (a Task 10 troca isso por `openSession`); sem `HQREADER_BOOK`, fica na biblioteca. A transição entre visões usa `AnimatePresence` com fade ~150 ms respeitando `useReducedMotion`.
- [ ] **Step 5: Restilizar o `Reader`** (linha de controles e contêineres usam tokens, texto do contador/zoom em `--paper-ink`; a **área da página** continua num fundo escuro neutro fixo `#0f0d0b`; o aviso `navError` usa tokens). Sem mudar nomes de botões nem a estrutura que `verify-reader-interactions.ps1` consome (`Anterior`, `Proxima`, `Pagina unica`/`Pagina dupla`, contador `N / M` e zoom `NNN%` em um nó de texto cada).
- [ ] **Step 6: Verificar.** `npm install` se preciso, `npm run build`, `npx -p typescript@5 tsc --noEmit -p tsconfig.json`. Rodar com `HQREADER_BOOK=<cbz de verificacao>` + `HQREADER_DATA_DIR` temporário e capturar por `PrintWindow` o leitor nos temas claro e escuro (clicando `Tema escuro` com mouse real) e a barra superior; conferir pela árvore UIA que os controles do leitor continuam com os mesmos nomes. Descrever o que se **viu**; comparar visualmente com os mockups (cores de fundo dos dois temas com amostragem de pixel: fundo ≈ `#f3ede2` / `#17130f` fora da área da página).
- [ ] **Step 7: Commit** (`git add cmd/hqreader/frontend`; mensagem `frontend: tema Papel claro/escuro/auto, barra superior e troca de visoes`).

---

### Task 8: Frontend — tela inicial, eventos de scan, adicionar/remover/atualizar

**Files:**
- Create: `src/store/libraryStore.ts`, `src/lib/scanEvents.ts`, `src/components/LibraryHome.tsx`, `src/components/RootCard.tsx`, `src/components/EmptyShelf.tsx`, `src/components/ConfirmDialog.tsx`, `src/components/ScanIndicator.tsx`
- Modify: `src/App.tsx`, `src/components/TopBar.tsx`

**Interfaces:**
- Consumes: bindings de `LibraryService` (Task 6): `ListRoots`, `AddLibrary`, `RemoveLibrary`, `StartScan`, `StartScanAll`, `CancelScan`; evento `library:scan` (formato confirmado na Task 1); `viewStore`, `themeStore`.
- Produces: `useLibraryStore` com `{ roots: RootSummary[]; loadRoots(): Promise<void>; addLibrary(): Promise<AddResult>; removeLibrary(rootID): Promise<void>; rescan(rootID): Promise<void>; rescanAll(): Promise<void>; cancelScan(rootID): Promise<void>; scans: Record<string, {found:number; validated:number; running:boolean}>; notice: {text:string}|null; dismissNotice(): void }`; `subscribeScanEvents()` em `src/lib/scanEvents.ts` (assina `Events.On("library:scan", ...)` uma vez, devolve a função de cancelamento; chama `applyScanEvent` da store) — esta task trata `started/walked/done/cancelled/error` (contadores e recarga das raízes); o merge de `batch` na pasta aberta é da Task 9; `ErrorCode` → texto em português (tabela de códigos) mostrado como aviso dispensável (mesmo vocabulário visual do `navError`).

- [ ] **Step 1: Store e eventos.** Implementar a store e a assinatura; `loadRoots` na montagem; recarregar raízes quando chega `walked`/`done`/`cancelled` e a cada 1 s durante scan (atualiza contagens); `rescanAll` chama `StartScanAll`.
- [ ] **Step 2: Estado vazio e lista de raízes** conforme o mockup `tela-inicial.html`: `EmptyShelf` (pilha de capas em gradiente, "Sua estante está vazia", frase explicando CBZ/CBR, botão `Adicionar biblioteca`); `RootCard` (nome, caminho, "N HQs · M sub-bibliotecas", estado: "N encontrados…" + barra fina durante scan, "Atualizado …" ou o aviso "scan incompleto" quando `ScannedAt == 0` e sem scan, botão `Opcoes da biblioteca` com menu contendo `Remover biblioteca`); barra de ações na tela inicial com `Adicionar biblioteca` e `Atualizar tudo`; `ScanIndicator` com `Cancelar scan` quando houver scan na raiz.
- [ ] **Step 3: Remover com confirmação** (`ConfirmDialog`: `role="dialog"`, `aria-modal`, foco inicial no `Cancelar`, `Escape` cancela, botões `Remover`/`Cancelar`), texto deixando claro que os arquivos do disco não são apagados.
- [ ] **Step 4: Verificar.** build + tsc. Com `HQREADER_DATA_DIR` novo e `HQREADER_PICK_FOLDER` apontando para uma pasta de CBZ (usar `cmd/hqreader/make-verify-cbz.go` ou criar poucos arquivos à mão): estado vazio visível (screenshot e árvore UIA), clique **real** em `Adicionar biblioteca` → cartão aparece com contagem subindo (amostrar duas vezes) → ao fim "N HQs"; adicionar a mesma pasta de novo mostra o aviso `exists` e adicionar uma subpasta mostra `inside_library`; menu `⋯` → `Remover biblioteca` → `Cancelar` mantém, `Remover` volta ao estado vazio; screenshots nos dois temas. Usar `HQREADER_SCAN_DELAY_MS=300` para ver o scan em andamento.
- [ ] **Step 5: Commit** (`git add cmd/hqreader/frontend`; mensagem `frontend: tela inicial com bibliotecas, scan em tempo real, adicionar e remover`).

---

### Task 9: Frontend — navegação estilo explorador, cartões, placeholder de capa

**Files:**
- Create: `src/lib/cover.ts`, `src/components/FolderView.tsx`, `src/components/Breadcrumb.tsx`, `src/components/FolderCard.tsx`, `src/components/BookCard.tsx`, `src/components/CoverPlaceholder.tsx`
- Modify: `src/store/libraryStore.ts`, `src/lib/scanEvents.ts`, `src/components/LibraryHome.tsx`, `src/components/RootCard.tsx`

**Interfaces:**
- Consumes: `LibraryService.ListFolder(libraryID) FolderView`, `StartScan`, eventos `batch` (`Books`, `Folders`), DTOs da Task 5.
- Produces: na `useLibraryStore`: `folder: FolderView | null`, `openFolder(id: string | null): Promise<void>` (`null` = tela inicial "Minhas HQs"; guarda `currentFolderID`), `refreshFolder(): Promise<void>` (consulta de novo a pasta atual), `mergeBatch(ev: ScanEvent)` (anexa/atualiza por `ID` os `Books` com `LibraryID == folder.ID` e as `Folders` com `ParentID == folder.ID`; agenda `refreshFolder` com throttle de 1 s durante o scan e uma vez no `done`); `coverStyle(seed: string): { c1: string; c2: string; initials: string }` em `cover.ts` (hash FNV-1a determinístico do seed escolhe um dos 8 pares de gradiente da tabela; `initials` = iniciais de até 2 palavras alfanuméricas, maiúsculas, `"?"` se vazio); `BookCard` (placeholder de capa, título, subtítulo, barra fina de progresso + texto `Novo`/`N / M`/`Lido`, estados `broken` com o motivo traduzido e `missing` esmaecido, desabilitados para abrir); `FolderCard` ("N HQs · M pastas"); botão `Atualizar` dentro de uma biblioteca (reescaneia a **raiz** da pasta atual: `StartScan(folder.RootID)`); `Breadcrumb` começando em `Minhas HQs`, itens clicáveis.

- [ ] **Step 1: `cover.ts` + scratch check.** Escrever o módulo puro e um check Node **descartável** (`node scratch-check.ts`, não commitar): mesmo seed → mesma saída; 8 seeds diferentes cobrem pelo menos 5 pares distintos; `initials("Os Fabulosos X-Men #175") == "OF"`, `initials("sandman") == "S"`, `initials("") == "?"`, `initials("  --  ") == "?"`. Apagar o scratch.
- [ ] **Step 2: Navegação.** Clicar num `RootCard` ou `FolderCard` chama `openFolder(id)`; `Breadcrumb` navega por `openFolder` do item (e `Minhas HQs` → `openFolder(null)`); ordem fixa: pastas primeiro, depois livros (a ordem já vem do backend). Pasta vazia mostra uma frase amigável.
- [ ] **Step 3: Chegada gradual.** Ligar `mergeBatch` ao handler de eventos; cartões novos entram com `motion` (fade + leve subida, 150–200 ms, desligado com `prefers-reduced-motion`), sem reanimar os já existentes (chave estável por `ID`). Cartões "esqueleto" (visual, não vêm do banco) enquanto o scan tem `Validated < Found`, no máximo 6, com o texto "Carregando…".
- [ ] **Step 4: Desempenho.** Cada cartão com `content-visibility: auto` e `contain-intrinsic-size` coerente com a altura do cartão; a grade usa CSS grid responsivo. Registrar no relatório o tempo para renderizar e rolar 1 200 cartões (ver Task 11 para a pasta grande); se houver engasgo perceptível, adicionar janelamento simples e dizer isso.
- [ ] **Step 5: Verificar.** build + tsc. Com um fixture de poucas pastas aninhadas (use o gerador da Task 11 se já existir, senão árvore à mão): entrar em sub-biblioteca com clique real, breadcrumb mostra `Minhas HQs › raiz › sub`, voltar pelo breadcrumb e pela raiz; um livro corrompido mostra o aviso; `HQREADER_SCAN_DELAY_MS=300` mostra os cartões chegando e o botão `Atualizar` reinicia o scan; screenshots nos dois temas conferidos contra o mockup (capa com gradiente e iniciais, barra de progresso, sombra chapada).
- [ ] **Step 6: Commit** (`git add cmd/hqreader/frontend`; mensagem `frontend: navegacao estilo explorador, cartoes de HQ e capas placeholder`).

---

### Task 10: Frontend — abrir da biblioteca, voltar, progresso e estado por livro

**Files:**
- Modify: `src/store/readerStore.ts`, `src/components/Reader.tsx`, `src/components/PageView.tsx`, `src/components/BookCard.tsx`, `src/hooks/useReaderShortcuts.ts`, `src/store/viewStore.ts`, `src/components/TopBar.tsx`
- Create: `src/hooks/useProgressReporter.ts`

**Interfaces:**
- Consumes: `LibraryService.OpenBook`, `OpenFile`, `CloseBook`, `SaveProgress` (Task 6), `OpenedBook` (Task 5), `reader.Service.SetCurrentPage` (já usado), `computeSpread`/`ReaderView` (Etapa 3c), `useViewStore`.
- Produces: no `readerStore`: campos novos `libraryBookID: string`, `title: string`, `furthestPage: number`; ação `openSession(opened: OpenedBook): void` substitui `loadCurrentBook` (zera tudo por livro: `zoom`, `pan`, `pageAspects`, `navError`, `error`, define `bookID = opened.Session`, `pageCount`, `currentPage = opened.StartPage`, `loading = false`, e chama `SetCurrentPage(session, StartPage)`); `closeSession(): Promise<void>` (chama `CloseBook(session, libraryBookID)` e volta ao estado inicial); `recordAspect(bookID, index, aspect)` passa a **ignorar** chamadas cujo `bookID` não é o da sessão atual; `useProgressReporter()` observa a visão **assentada** (`computeSpread(...).pending === false`) e chama `SaveProgress(libraryBookID, firstIndex, lastIndex)` quando o par `[first,last]` muda, ignorando sessões sem `libraryBookID` (atalho de dev). `BookCard` clicável chama `openBook(id)` → trata `ErrorCode` (aviso dispensável com o texto do código) ou `openSession` + `enterReader`. `Voltar para a biblioteca` chama `closeSession` + `exitReader` e **mantém** a pasta atual da `libraryStore` (e refaz a consulta para atualizar o progresso nos cartões).

- [ ] **Step 1: `readerStore`.** Implementar `openSession`/`closeSession`, mudar `recordAspect`, remover `loadCurrentBook`/`Current()` e seus usos (inclusive o `useEffect` de `Reader`). `Reader` deixa de ter estado de "carregando" próprio; PageView recebe `key={bookID}` para zerar `failed`/`retries` por livro.
- [ ] **Step 2: Progresso.** `useProgressReporter` montado no `Reader`; o primeiro relatório após abrir conta como "começado" (Decisão 6). Garantir que `togglePageMode` e a resolução de `pending` geram o relatório correto (ex.: par final `[n-2, n-1]` reporta `furthest = n-1` só depois de assentado).
- [ ] **Step 3: Abrir e voltar.** Ligar `BookCard` → abrir (livros `missing`/`broken` não abrem); leitor mostra o título na barra superior; `Voltar para a biblioteca` na barra; atalhos do leitor só agem quando `view === "reader"`, há `bookID` e não há `error` (fecha a pendência de gate), e `m` ignora `e.repeat`.
- [ ] **Step 4: `Reader.tsx` com seletores** do Zustand (um `useReaderStore(s => ...)` por campo ou `useShallow`) em vez de desestruturar a store inteira.
- [ ] **Step 5: Verificar.** build + tsc. Com fixture e `HQREADER_DATA_DIR` temporário: abrir um livro por clique real, avançar por teclado até uma página conhecida, `Voltar para a biblioteca` volta **na mesma pasta** e o cartão mostra `N / M`; reabrir o mesmo livro cai na mesma página (contador); abrir outro livro depois não herda zoom/pan/aspectos do anterior (zoom do segundo = `100%`, modo `Pagina unica`); fechar o app **depois** de virar uma página e **antes** do debounce (matar o PID do app logo após a virada) e reabrir: o progresso não pode ter regredido além do que o `CloseBook`/`ServiceShutdown` conseguem descarregar (registrar o que foi observado; perda de até o debounce no kill brusco é aceita e documentada). Rodar `verify-reader-interactions.ps1` **uma vez** em `wails3 dev` com `HQREADER_BOOK` como regressão rápida do leitor e anotar o resultado (a rodada oficial vem na Task 11).
- [ ] **Step 6: Commit** (`git add cmd/hqreader/frontend`; mensagem `frontend: abrir da biblioteca, voltar, progresso por livro e retomada na pagina salva`).

---

### Task 11: Verificação de ponta a ponta, regressão do leitor e documentação

**Files:**
- Create: `cmd/hqreader/make-verify-library.go` (`//go:build ignore`), `cmd/hqreader/verify-library.ps1`, `cmd/hqreader/verify-library-dev-output.txt`, `cmd/hqreader/verify-library-prod-output.txt`, `cmd/hqreader/verify-reader-interactions-etapa4-dev-output.txt`, `cmd/hqreader/verify-reader-interactions-etapa4-prod-output.txt`
- Modify: `cmd/hqreader/MANUAL_VERIFICATION.md` (acrescentar seção), `docs/superpowers/specs/2026-09-16-hq-reader-design.md` (§3.3: `Open` do CBR extrai tudo e o scanner usa `Inspect`; §3.5: tabela `libraries`, `books.library_id`, `furthest_page`; §6: marcar a Etapa 4 como feita), `cmd/hqreader/verify-reader-interactions.ps1` (somente se algum nome de controle mudou; preferir não mexer)

**Interfaces:**
- Consumes: o app completo (Tasks 1–10), `make-verify-cbz.go` (CBZ de 10 páginas com faixa branca/magenta e página 7 corrompida), as variáveis de ambiente da tabela.
- Produces: fixture reproduzível (`go run make-verify-library.go <pasta> [--big] [--mutate]`), script real e completo, saídas brutas de duas execuções (dev e prod), regressão do leitor em dev e prod.

- [ ] **Step 1: Gerador do fixture.** `make-verify-library.go`: cria `<pasta>/Marvel/{Classicos,Moderna/Anos 2000}/...` e `<pasta>/Independentes/...` com ~40 CBZ de poucas páginas PNG reais (cada uma com a faixa branca do meio para checar a página), nomes com espaços e acentos (`Édição Especial #1.cbz`), alguns com `ComicInfo.xml` (título/série/número), um CBZ **corrompido** (bytes que não são zip) e um `.txt` ignorado; `--big` acrescenta `Marvel/Gigante/` com 1 200 CBZ de uma página (para a checagem de pasta grande); `--mutate` apaga um arquivo conhecido e cria um novo (para o rescan). Saída imprime a lista de caminhos esperados. Confirmar com `go run ./cmd/hqcli list` em alguns.
- [ ] **Step 2: Script `verify-library.ps1`** (real, completo, re-executável; padrões das etapas 2–3c: localizar janela por PID, restaurar/focar, helpers UIA por nome, cliques e teclas **reais**, `PrintWindow` para pixels, contadores PASS/FAIL, saída `=== SUMMARY: N passed, M failed ===`, exit 0/1). Parâmetros: `-ProcessName`, `-FixtureDir`, `-DataDir`, `-Phase` (o script roda em fases porque o app é reiniciado no meio: o chamador relança o app com o mesmo `HQREADER_DATA_DIR` entre as fases; documentar o procedimento no cabeçalho). Antes de escrever as asserções, **inspecionar a árvore UIA real** (nomes dos cartões, do breadcrumb, dos botões, do texto de contagem) e registrar o achado no relatório; geometria/cor por pixel (a UIA não reflete transforms). Cenários (cada um com asserções que podem falhar):
  1. **Estado vazio** (`DataDir` novo): texto "Sua estante está vazia" e botão `Adicionar biblioteca`; nenhum cartão de raiz.
  2. **Adicionar** (`HQREADER_PICK_FOLDER=<fixture>/Marvel`, `HQREADER_SCAN_DELAY_MS=150`): clique real; cartão `Marvel` aparece; durante o scan, o texto "N encontrados" existe e **N cresce** entre duas amostras; entrar no cartão **durante** o scan (clique real) e ver cartões de livro **aumentando** entre duas amostras; abrir um livro durante o scan (clique real) mostra o leitor com a página 1 e o botão `Voltar para a biblioteca`; voltar cai na **mesma pasta**.
  3. **Scan concluído:** indicador some; cartões de pasta (`Classicos`, `Moderna`) com "N HQs · M pastas"; o CBZ corrompido tem o aviso de livro quebrado e **clicar nele não abre o leitor** (continua na biblioteca); um título vindo de `ComicInfo.xml` aparece no lugar do nome do arquivo; nome com acento exibido correto.
  4. **Navegação:** clique real em `Moderna` → `Anos 2000`; breadcrumb contém `Minhas HQs`, `Marvel`, `Moderna`, `Anos 2000` (nós de texto/botões); clique em `Marvel` no breadcrumb volta; `Minhas HQs` volta à tela inicial.
  5. **Progresso e retomada:** abrir um livro, ir até a página 5 (teclado), `Voltar para a biblioteca`, cartão mostra `5 / N`; **encerrar o app por PID**, relançá-lo com o mesmo `DataDir`, abrir o mesmo livro: contador do leitor `5 / N`. Chegar à última página de outro livro e voltar: cartão `Lido`. Abrir o terceiro livro: zoom `100%` e `Pagina unica` (nada herdado).
  6. **Rescan:** `--mutate` no disco; clique real em `Atualizar` dentro de `Marvel`; o arquivo apagado vira cartão esmaecido/`missing` (texto "ausente" ou o definido) e o novo aparece; um scan cancelado (`Cancelar scan` clicado cedo com delay alto) **não** marca nada como ausente (comparar antes/depois).
  7. **Pasta grande** (`--big`): scan sem delay; a interface continua respondendo (um clique em `Voltar`/breadcrumb funciona em < 2 s durante e depois do scan); o texto "1200 HQs" aparece na pasta `Gigante`; tempo de rolagem/render registrado (sem enumerar 1 200 nós UIA).
  8. **Sobreposição e remoção:** adicionar de novo a mesma pasta → aviso de "já cadastrada"; adicionar `Marvel/Classicos` → aviso de "dentro de uma biblioteca"; menu `Opcoes da biblioteca` → `Remover biblioteca` → `Cancelar` mantém; remover **durante** um scan (delay alto) → volta ao estado vazio sem erro e sem processos presos.
  9. **Tema:** clique real em `Tema escuro`: amostragem de pixel do fundo fora das áreas de conteúdo ≈ `#17130f` (±6 por canal) e em `Tema claro` ≈ `#f3ede2`; `Tema do sistema` pressionado por padrão em `DataDir` novo (`aria-pressed`/estado UIA conforme a árvore real); a escolha persiste após reiniciar o app.
  10. **Nada aconteceu com o disco:** depois de remover a biblioteca, todos os arquivos do fixture continuam existindo.
- [ ] **Step 3: Dry runs com parcimônia, depois as duas execuções oficiais.** Dry run(s) até o script passar de ponta a ponta; então **uma** execução em `wails3 dev` e **uma** em `wails3 build` (`bin\hqreader.exe`), cada uma completa, do zero (novo `DataDir` temporário), gravando a saída bruta com `*>` em `verify-library-dev-output.txt` e `verify-library-prod-output.txt`. Cada arquivo termina em `=== SUMMARY: N passed, 0 failed ===`; os PIDs e a linha de processos-pai (como na Etapa 3c) diferem entre os dois. Falha por defeito do app: não enfraquecer a asserção; reportar com a saída real. Falha por suposição errada do script: corrigir o script e **refazer as duas execuções do zero**.
- [ ] **Step 4: Regressão do leitor.** Rodar `verify-reader-interactions.ps1` com `HQREADER_BOOK` + `HQREADER_DATA_DIR` temporário, uma vez em dev e uma vez em prod, salvando `verify-reader-interactions-etapa4-dev-output.txt` e `-prod-output.txt`; ambos devem passar todas as verificações (a linha de resumo da Etapa 3c era `144 passed, 0 failed`; hoje deve dar o mesmo número, ou explicar a diferença).
- [ ] **Step 5: Verificação manual do diálogo nativo.** O diálogo de pasta do Windows não é automatizado: abrir o app **sem** `HQREADER_PICK_FOLDER`, clicar `Adicionar biblioteca`, escolher uma pasta no diálogo real, confirmar que a biblioteca é adicionada, e repetir cancelando o diálogo (nada acontece, sem erro). Registrar exatamente isso (e que foi feito à mão) em `MANUAL_VERIFICATION.md`.
- [ ] **Step 6: Documentação.** Acrescentar (não substituir) a seção "Etapa 4" em `MANUAL_VERIFICATION.md` com: data, procedimento, PIDs, linhas de resumo copiadas dos arquivos, checklist marcado **só** onde há `PASS` nos dois arquivos, observações e limitações (diálogo nativo manual; progresso perdido até o debounce em kill brusco; WebKitGTK não validado; pareamento do modo duplo inalterado). Atualizar o spec mestre (§3.3, §3.5, §6) como descrito em Files.
- [ ] **Step 7: Commit** (`git add cmd/hqreader docs`; mensagem `hqreader: verificacao de ponta a ponta da biblioteca (dev e producao), regressao do leitor e docs da Etapa 4`).

---

## Self-Review

**Cobertura da spec:** §5.0 tela inicial (Task 8; estado vazio, cartões, remover, atualizar tudo) · §5.1 bibliotecas/sub-bibliotecas, remoção sem tocar arquivos, regra de sobreposição (Tasks 4, 5, 8) · §5.2 scanner (`Inspect` Task 2; duas fases, lotes, cancelamento, `missing`/`broken`, scan interrompido, um scan por raiz: Tasks 4 e 5) · §5.3 abrir/ler, voltar, `broken` com motivo, `HQREADER_BOOK` (Tasks 5, 10) · §5.4 cartões e ordem (Tasks 4 `ToBookItem`, 9) · §5.5 progresso, debounce, retomada, "Lido" por `furthest_page`, modo duplo reporta o último índice da visão (Tasks 3, 5, 10) · §6 visual Papel claro/escuro/auto, tokens, tipografia, formas, tema em `localStorage`, escopo (leitor com área da página neutra), animações discretas (Tasks 7, 9) · §7 arquitetura (Tasks 3–6) · §8 erros (banco futuro, falha de progresso não interrompe a leitura: `SaveProgress` do frontend ignora erro; avisos dispensáveis) · §10 verificação (testes Go das Tasks 2–5; script real + saídas dev/prod + diálogo manual na Task 11) · §11 riscos (Task 1: API do Wails, desempenho do `modernc`, custo do CBR, caminhos) · pendências da Etapa 3c (estado por livro e `recordAspect` com `bookID`, abertura na página salva, gate dos atalhos em `!error`, `m` com repeat, `Reader` com seletores: Task 10).

**Lacuna conhecida e aceita:** a perda de até `ProgressDebounce` (750 ms) de progresso num encerramento brusco do processo (kill) é aceita e documentada (Task 10, Step 5; Task 11, Step 6). O pareamento do modo duplo continua o da Etapa 3c.

**Consistência de tipos:** `FolderItem`/`BookItem`/`ScanEvent` são declarados na Task 4 e usados na 5 (DTOs de `FolderView`), 6 (bindings) e 8–10 (frontend); `Status ∈ {"new","reading","read"}` e os códigos de erro vêm das tabelas do cabeçalho; `OpenedBook.Session` é o `bookID` de sessão do `reader.Service` (usado por `SetCurrentPage`, `/page/{session}/{n}` e `CloseBook`), e `LibraryBookID` é o id persistente (usado por `SaveProgress`); `Service.CloseBook(session, libraryBookID)` tem a mesma assinatura na Task 5 e na Task 10; nomes de controles da UI só existem na tabela "Strings de UI" e são os mesmos usados pelo script da Task 11.

**Proporção:** corpo de código só onde a assinatura e os testes não determinam (nenhum algoritmo longo transcrito); cada passo diz o nome do teste e o que ele afirma.
