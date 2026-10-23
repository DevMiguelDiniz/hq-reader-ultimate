# Spike 06 — API real do Wails, desempenho do modernc, custo de CBR, NormalizePath (Etapa 4, Task 1)

Ambiente: Windows 11 Pro 26200, `go1.27.1 windows/amd64`, `wails3 v3.0.0-beta.23`,
`@wailsio/runtime 3.0.0-beta.23` (pinado no `package.json`), WebView2 155.0.4283.62,
node v24.17.0, sem compilador C (`CGO_ENABLED=0`). Código em `spikes/06-etapa4-apis/`
(módulo Go próprio; `go list -e ./...` na raiz não lista nenhum pacote de `spikes`).
Todo trecho entre crases abaixo foi copiado do output real das execuções; os logs
completos estão no relatório da task.

## ⚠ Achados que contradizem ou corrigem a spec/o plano

1. **Cancelar o diálogo de pasta devolve ERRO, não `("", nil)`.** `PromptForSingleSelection()`
   devolve `path=""` **e** `err = "cancelled by user"` (tipo `*errors.errorString`, o
   sentinela `cfd.ErrorCancelled` de um pacote **internal** do Wails — não dá para
   usar `errors.Is`). No JS a promise do binding **rejeita** com `RuntimeError`,
   `message: "cancelled by user"`. Quem implementar `PickFolder`/`AddLibrary`
   (Tasks 6/8) precisa traduzir esse erro para `ErrorCode: "cancelled"`
   comparando `err.Error() == "cancelled by user"`, senão o cancelamento vira
   exceção no frontend.
2. **3 dos 5 `.cbr` reais desta máquina são ZIP** (assinatura `PK`). O
   `archive.Open` atual escolhe o formato pela extensão e devolve
   `ErrCorrupted` para eles; o `Inspect` do plano (Task 2: "`Format` por
   extensão") herdaria isso e o scanner marcaria esses livros como `broken`.
   Os mesmos bytes com nome `.cbz` abrem normalmente (12–17 páginas). Sugestão:
   `Inspect`/`Open` farejam os bytes mágicos (`PK\x03\x04` → zip,
   `Rar!\x1a\x07` → rar) e só usam a extensão como desempate.
3. **DSN com prefixo `file:` quebra em caminho com `#`.** Com
   `file:<caminho>?_pragma=...` e uma pasta `Pasta Ação com espaço #1`, os pragmas
   foram aplicados, mas o banco foi criado **no lugar errado**: um arquivo
   chamado `Pasta Ação com espaço` na pasta-mãe (o `#` virou fragmento de URI).
   Caminho puro sem `file:` + `?_pragma=...` funcionou com `#`, acentos,
   espaços, `%20` e `&`. Usar a forma sem `file:` (ver item (e)).
4. **Transação "lê e depois escreve" falha com `SQLITE_BUSY` havendo outro
   escritor**, mesmo com `busy_timeout(5000)`: 925 de 994 e 208 de 250
   transações de progresso (`BEGIN; SELECT; UPSERT; COMMIT`) falharam com
   `database is locked (5) (SQLITE_BUSY)` enquanto o scan gravava lotes.
   Um `sync.Mutex` do Go em volta de toda transação de escrita zerou os erros
   e manteve os dois escritores andando. Isso afeta o `SaveProgress` (Task 3:
   "livro inexistente → `ErrNotFound`" sugere ler antes de gravar) concorrente
   com `UpsertBooks` do scan. Detalhes em (f).
5. **`modernc.org/sqlite` mais recente (v1.61.0) exige `go 1.26.0`**: `go get`
   subiu a diretiva `go 1.25.0 => 1.26.0` e `golang.org/x/sys v0.46.0 => v0.49.0`
   (dependência do Wails). O próprio Wails beta.23 já puxa
   `modernc.org/sqlite@v1.44.3` (que pede `go 1.24.0`) e ela se comportou igual
   em todos os pontos funcionais. A Task 3 escolhe: v1.44.3 mantém `go 1.25.0`;
   v1.61.0 sobe o piso (o plano já aceita isso).
6. **Evento emitido dentro de `ServiceStartup` se perde**: o frontend ainda não
   existe (`startup-ping seen=false` no dev e no exe de produção). O frontend
   deve buscar o estado ao carregar; não contar com eventos de startup.

## Veredito por item

| Item | Veredito |
|---|---|
| (a) diálogo de pasta | **GO COM AJUSTE** (cancelar = erro "cancelled by user") |
| (b) eventos de goroutines | **GO** |
| (c) bindings | **GO** (campos com o nome Go exato; `int64` vira `number`) |
| (d) `ServiceStartup`/`ServiceShutdown` | **GO COM AJUSTE** (evento emitido no startup se perde) |
| (e) DSN | **GO COM AJUSTE** (sem prefixo `file:`) |
| (f) desempenho/concorrência | **GO COM AJUSTE** (mutex de escrita no store) |
| (g) custo de CBR | **GO COM AJUSTE** (farejar ZIP com extensão `.cbr`; nenhuma amostra sólida) |
| (h) `NormalizePath` | **GO** (Decisão 9 como está; limites anotados) |

---

## (a) Diálogo nativo de pasta — GO COM AJUSTE

Código que funcionou (`service.go`), num método de serviço chamado pelo binding:

```go
path, err := application.Get().Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
    CanChooseDirectories: true,
    CanChooseFiles:       false,
    Title:                "Escolha uma pasta (spike06)",
    Directory:            os.Getenv("SPIKE_PICK_DIR"), // pasta inicial; vazio = padrão do Windows
}).PromptForSingleSelection()
```

Os nomes do plano batem com a beta.23. No Windows, com `CanChooseDirectories:true`
o Wails abre `cfd.NewSelectFolderDialog` (o valor de `CanChooseFiles` é ignorado)
e devolve `filepath.Clean(path)` (`dialogs_windows.go`).

**Cancelar** (clique de mouse real no botão "Escolher pasta" da janela, `{ESC}` por
SendKeys ~1 s depois, `wails3 dev`):

```
real click on 'Escolher pasta' at (618,383)
-- windows of pid 5908 (about 1 s after the click):
   name='Escolha uma pasta (spike06)' class='#32770' hwnd=0x308D4
   name='spike06' class='WailsWebviewWindow' hwnd=0x63077A
-- foreground: hwnd=0x308D4 pid=5908 class='#32770' title='Escolha uma pasta (spike06)'
sent {ESC}
16:21:46.026 pid=5908 PickFolder: returned path="" (len=0) err=cancelled by user errType=*errors.errorString errNil=false elapsed=1.2072195s
16:21:46.037 pid=5908 JS: pickout pick REJECTED ctor=RuntimeError name=RuntimeError message="cancelled by user" cause={} json={"name":"RuntimeError","cause":{}} after=1220ms
```

O diálogo abriu em primeiro plano sem `Window` definido (o `{ESC}` chegou nele).
A fonte do erro: `internal/go-common-file-dialog/cfd/errors.go:6: ErrorCancelled = errors.New("cancelled by user")`.

**Selecionar** (exe de produção, `SPIKE_PICK_DIR` = pasta com acento e espaço; o
diálogo abriu nela e o botão OK foi apertado por `WM_COMMAND IDOK`, porque o
botão "Selecionar pasta" aparece na UIA como `Pane` sem nenhum padrão):

```
16:24:47.536 pid=17120 PickFolder: returned path="C:\\Users\\mfdin\\AppData\\Local\\Temp\\claude\\...\\scratchpad\\Pasta Ação com espaço" (len=135) err=<nil> errType=<nil> errNil=true elapsed=12.0802255s
16:24:47.549 pid=17120 JS: pickout pick RESOLVED value="C:\\Users\\...\\Pasta Ação com espaço" typeof=string after=12100ms
```

Sucesso = caminho absoluto com `\`, sem separador final, acentos intactos, `err=nil`.
A opção `Directory` funciona como pasta inicial.

Nota de honestidade: durante a execução em `wails3 dev` houve mais duas chamadas
de `PickFolder` que **não** vieram do script (alguém usando a máquina): uma às
16:21:38 cancelada após 4,5 s, e uma às 16:22:03 que devolveu
`path="H:\\Hq" err=<nil>` (seleção manual). Elas só confirmam o mesmo formato.

Não testado: o diálogo com `Window: application.Get().Window.Current()` (modal à
janela); sem ela, a janela principal pode continuar clicável com o diálogo aberto
(não verificado).

## (b) Eventos de goroutines — GO

```go
application.Get().Event.Emit("burst", BurstEvent{Worker: w, Seq: i, Global: g, Path: ..., Ok: true})
```

```ts
import { Events } from "@wailsio/runtime";
Events.On("burst", (e) => { const d = e.data; /* d.Worker, d.Seq, d.Path ... */ });
```

1000 eventos emitidos por 8 goroutines simultâneas (`EmitBurst(1000)`), contados no frontend:

```
16:23:19.269 EmitBurst: all 1000 Emit calls returned in 9.0824ms
16:23:21.360 JS: burstout burst received=1000 expected=1000 unique=1000 dupes=0 missing=0 perWorkerOutOfOrder=0 globalOutOfOrder(vs emit counter)=80 doneArrivedAfter=1000 spanFirstToLast=85ms workers=8
JS: burstout burst firstEventShape={"keys":["name","data"],"ctor":"WailsEvent","name":"burst","data":{"Worker":7,"Seq":0,"Global":1,"Path":"C:\\Comics\\Ação 7\\0.cbz","Ok":true},"typeofData":"object"}
```

Exe de produção: `all 1000 Emit calls returned in 7.411ms`, `received=1000 ... dupes=0 missing=0 perWorkerOutOfOrder=0 ... spanFirstToLast=86ms` (classe minificada `vt`).

- Nenhuma perda nem duplicata; a ordem dentro de cada goroutine é mantida.
  `globalOutOfOrder` só mede a ordem em relação a um contador incrementado antes
  do `Emit` (não atômico com o `Emit`), não indica perda.
- `Emit` não bloqueia (1000 chamadas em 7–9 ms). Na fonte, `EventProcessor.Emit`
  manda para uma `mailbox` que serializa a entrega ao frontend.
- Segurança concorrente: 8 goroutines emitindo ao mesmo tempo sem pânico nem
  perda. Não verificado com `-race` (sem CGO nesta máquina).
- Formato no JS: `e` é `WailsEvent` com chaves `name` e `data` (sem `sender` em
  eventos vindos do Go); `e.data` é o objeto do struct com os **nomes de campo
  Go exatos** (`Worker`, `Seq`, `Path`, `Ok`), já que o struct não tem tags json.
- Tipagem: sem registro, `e.data` é `any`. Com
  `application.RegisterEvent[BurstEvent]("burst")` em `init()`, o gerador emite o
  modelo `BurstEvent` e `eventdata.d.ts` aumenta `CustomEvents` com
  `"burst": main$0.BurstEvent`. Prova com tsc 5 num arquivo descartável:
  `src/zz_typecheck_scratch.ts(2,35): error TS2322: Type 'number' is not assignable to type 'string'.`
  para `const s: string = e.data.Worker` no evento registrado; o mesmo uso num
  evento não registrado passou sem erro. O `Emit` de evento registrado também
  valida o tipo do payload em tempo de execução (fonte: `validateCustomEvent`).

## (c) Bindings — GO

Comando que regenera **sem abrir janela** (rodado na pasta do módulo; 10,3 s e 7,2 s):

```
wails3 generate bindings -clean=true -ts -i
Processed: 259 Packages, 1 Service, 5 Methods, 0 Enums, 4 Models, 1 Event in 7.2271434s.
```

(No `wails3 build` o Taskfile chama o mesmo com `-f '<BUILD_FLAGS>'`, ex.:
`wails3 generate bindings -f '-tags production -trimpath -buildvcs=false -ldflags="-w -s -H windowsgui"' -clean=true -ts -i`.)
Só rodei no spike; em `cmd/hqreader` o comando é o mesmo, na pasta dele.

Arquivos: `frontend/bindings/<caminho do módulo>/{index.ts,models.ts,spikeservice.ts}`
e `frontend/bindings/github.com/wailsapp/wails/v3/internal/{eventcreate.ts,eventdata.d.ts}`.
`ServiceStartup`/`ServiceShutdown` **não** viram bindings (5 métodos = PickFolder,
EmitBurst, Report, Sample, Slow). Assinaturas geradas:

```ts
export function PickFolder(): $CancellablePromise<string>
export function EmitBurst(n: number): $CancellablePromise<number>
export function Sample(): $CancellablePromise<$models.SampleResult>
export function Slow(ms: number): $CancellablePromise<number>
export function Report(line: string): $CancellablePromise<void>
```

Com `-i` os modelos são `interface` (sem classe nem `createFrom`):

```ts
export interface SampleResult {
    "ID": number;            // int64
    "Title": string;
    "Done": boolean;
    "Count": number;         // int
    "Ratio": number;
    "Inner": Inner;          // struct aninhado
    "InnerPtr": Inner | null;  // ponteiro
    "FilledPtr": Inner | null;
    "Items": Item[] | null;  // slice
    "NilItems": Item[] | null;
    "Tags": { [_ in string]?: number } | null;  // map
    "taggedField": string;   // `json:"taggedField"`
    "omittedField"?: string; // `json:",omitempty"`
}
```

Valor recebido no JS (`Sample()`, dev e produção iguais):

```
sample json={"ID":9007199254740992,"Title":"Edição Especial","Done":true,"Count":3,"Ratio":0.5,"Inner":{"Label":"inner","Items":[{"Name":"a.png","Size":10}]},"InnerPtr":null,"FilledPtr":{"Label":"filled","Items":null},"Items":[{"Name":"x","Size":1},{"Name":"y","Size":2}],"NilItems":null,"Tags":{"k":1},"taggedField":"t"}
sample ctor=Object keys=ID,Title,Done,Count,Ratio,Inner,InnerPtr,FilledPtr,Items,NilItems,Tags,taggedField typeofID=number ... ID.toString=9007199254740992 InnerPtr===null? true NilItems=null hasOmitted=false hasUnexported=false
```

- Ponteiro nil → `null`; slice nil → `null` (não `[]`) — o frontend precisa
  tratar `null` em listas (ou o Go devolver slice vazio não-nil).
- `int64` = `number`: o Go mandou `9007199254740993` (2^53+1) e o JS recebeu
  `9007199254740992`. Seguro para timestamps em segundos/ms; ids continuam string.
- Campo não exportado some; `omitempty` vazio some.
- Chamadas concorrentes são atendidas em paralelo:
  `par Sample resolved at 20ms, Slow(1500) resolved at 1520ms -> concurrent=true`.

## (d) `ServiceStartup` / `ServiceShutdown` — GO COM AJUSTE

```go
func (s *SpikeService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error
func (s *SpikeService) ServiceShutdown() error
```

`wails3 dev`:

```
16:21:22.305 main: calling app.Run
16:21:24.452 ServiceStartup: called; application.Get()!=nil=true; options.Name="" options.Route=""; windows=1
16:21:26.949 JS: startup startup-ping seen=false
...  (fechar a janela por WindowPattern.Close())
16:23:23.768 ServiceShutdown: called; startCtx.Err()=context canceled
16:23:23.770 main: app.Run returned err=<nil>
16:23:23.768 ServiceStartup ctx: Done (err=context canceled)
process 5908 exited within 15 s: True
```

Exe de produção: `ServiceStartup` 3 ms depois de `app.Run` (`16:24:31.670` → `16:24:31.673`, `windows=1`),
`startup-ping seen=false`, e ao fechar a janela `ServiceShutdown: called; startCtx.Err()=context canceled`
seguido de `app.Run returned err=<nil>`.

- `ServiceStartup` roda dentro de `app.Run`, antes do frontend carregar (no dev
  ele espera o servidor do Vite conectar: ~2,1 s), com a janela já criada e
  `application.Get()` disponível.
- Fechar a última janela encerra o app: o `ctx` do startup é cancelado **antes**
  de `ServiceShutdown`, que roda antes de `app.Run` retornar. Bom lugar para
  descarregar o debounce de progresso e fechar o banco.
- Evento emitido no `ServiceStartup` não chega ao frontend (contradição 6).
- Não testado: encerramento por kill do processo (obviamente sem shutdown).

## (e) DSN do modernc — GO COM AJUSTE

DSN confirmada (sem `file:`):

```go
db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
```

Os pragmas valem em **cada** conexão do pool (3 conexões abertas ao mesmo tempo com `db.Conn`):

```
plain path+_pragma (accents/space/#): conn#0 journal_mode=wal foreign_keys=1 busy_timeout=5000 errs=<nil>/<nil>/<nil>
plain path+_pragma (accents/space/#): conn#1 journal_mode=wal foreign_keys=1 busy_timeout=5000 errs=<nil>/<nil>/<nil>
plain path+_pragma (accents/space/#): conn#2 journal_mode=wal foreign_keys=1 busy_timeout=5000 errs=<nil>/<nil>/<nil>
plain path+_pragma (accents/space/#): insert book with unknown library_id -> err=constraint failed: FOREIGN KEY constraint failed (787)
plain path+_pragma (%20 & in name): conn#0 journal_mode=wal foreign_keys=1 busy_timeout=5000 ...
  files created in "...\\db\\Pasta Ação com espaço #1": [dsn2.db]
  files created in "...\\db\\a%20b & c": [dsn3.db]
  entries in -dir itself: ["Pasta Ação com espaço" "Pasta Ação com espaço #1" "a%20b & c"]
```

O `file:` + `#` (contradição 3): `dsn.db` nunca apareceu na pasta pedida; a entrada
`Pasta Ação com espaço` na pasta de cima é o banco criado no lugar errado
(32 768 bytes). Na rodada com modernc v1.44.3 os pragmas e o erro de FK saíram
iguais, mas a listagem de arquivos foi filtrada da saída — o local do arquivo
com `file:` + `#` só foi observado na v1.61.0.
Opção extra útil: `&_txlock=immediate` existe e funciona (ver (f)), mas não é a recomendação.

## (f) Desempenho e concorrência — GO COM AJUSTE

`go build` com `CGO_ENABLED=0`: OK (`build CGO_ENABLED=0` no `go version -m`). Banco
em pasta temporária fora do repo. Schema no formato de `books` da Task 3,
UPSERT com statement preparado por transação.

**Inserir 5 000 livros** (3 execuções cada, v1.61.0; segunda rodada completa):

```
batch= 100: [172ms 175ms 159ms]   | rodada 2: [173ms 161ms 161ms]
batch= 500: [66ms 66ms 69ms]      | rodada 2: [70ms 75ms 80ms]
batch=1000: [55ms 74ms 58ms]      | rodada 2: [87ms 64ms 63ms]
re-upsert of the same 5000 ids (batch=500): 61ms | 72ms
```

v1.44.3: `batch=100: [153 154 153]ms`, `batch=500: [65 77 70]ms`, `batch=1000: [57 56 61]ms`.
Lotes de 500 já pegam quase todo o ganho.

**4 leitores em loop + 1 escritor** (10 000 linhas, lotes de 100; leitores
alternam listagem com JOIN em `reading_progress` e `count(*)`):

```
run1 free pool + busy_timeout           writer 847ms  | readers: 599 queries, locked=0 busy=0 other=0, p50=2.582ms p99=29.096ms max=81.754ms
run1 SetMaxOpenConns(1) + busy_timeout  writer 2.251s | readers: 464 queries, locked=0 busy=0 other=0, p50=14.688ms p99=88.142ms max=124.549ms
run1 free pool, NO busy_timeout         writer 661ms  | readers: 513 queries, locked=0 busy=0 other=0, p50=2.67ms p99=23.888ms max=58.568ms
run2 free pool + busy_timeout           writer 533ms  | readers: 377 queries, ... p50=3.627ms p99=26.59ms
run2 SetMaxOpenConns(1) + busy_timeout  writer 1.995s | readers: 429 queries, ... p50=14.073ms p99=68.327ms
run2 free pool, NO busy_timeout         writer 2.347s | readers: 2030 queries, ... p50=3.011ms p99=18.681ms
```

(primeira rodada da sessão: free pool p50=2.012ms, `SetMaxOpenConns(1)` p50=12.545ms, writer 416ms vs 1.339s.)
Leitores com WAL nunca deram `locked`/`busy`, nem sem `busy_timeout`.
**`SetMaxOpenConns(1)` não é necessário para leitura** e piora: leitor 4–6× mais
lento no p50 e escritor 2,7–3,7× mais lento. Os tempos variam bastante entre
rodadas (máquina em uso); as conclusões se repetem em todas.

**Dois escritores** (scan gravando lotes + 2 goroutines de progresso fazendo
`BEGIN; SELECT existe?; UPSERT; COMMIT` a cada 2 ms):

```
run1 deferred tx (default)            scan 2.769s | progress: 994 tx, locked=925 first="database is locked (5) (SQLITE_BUSY)"
run1 _txlock=immediate                scan 313ms  | progress: 2 tx, locked=0
run1 SetMaxOpenConns(1)               scan 535ms  | progress: 159 tx, locked=0
run1 Go write mutex, deferred         scan 450ms  | progress: 104 tx, locked=0
run1 progress=single autocommit UPSERT scan 371ms | progress: 17 tx, locked=0
run2 deferred tx (default)            scan 463ms  | progress: 250 tx, locked=208
run2 _txlock=immediate                scan 325ms  | progress: 2 tx, locked=0
run2 SetMaxOpenConns(1)               scan 542ms  | progress: 159 tx, locked=0
run2 Go write mutex, deferred         scan 463ms  | progress: 106 tx, locked=0
run2 progress=single autocommit UPSERT scan 334ms | progress: 12 tx, locked=0
```

- Transação adiada que lê e depois escreve recebe `SQLITE_BUSY` **na hora** quando
  outro escritor commitou depois do início dela (o `busy_timeout` não ajuda).
- `_txlock=immediate` e UPSERT único em autocommit evitam o erro, mas o
  escritor de progresso quase não consegue vez (2 e 12–17 transações em ~0,3 s):
  num scan longo e sem pausas ele dependeria do `busy_timeout` de 5 s.
- **Recomendação para a Task 3:** pool livre (sem `SetMaxOpenConns(1)`), DSN do
  item (e), e um `sync.Mutex` do store em volta de **toda** transação/escrita
  (0 erros, os dois escritores avançam). `TestConcurrentReadDuringWrite` do plano
  deve passar com isso; vale um teste extra de escritores concorrentes.

**Tamanho do binário** (`wails3 build`, `-tags production -trimpath -ldflags="-w -s -H windowsgui"`, `CGO_ENABLED=0`):
spike Wails sem SQLite 10 306 048 bytes; com `modernc.org/sqlite v1.61.0`
linkado (`sqlite_link.go`) **14 152 192 bytes** (+3,85 MB). O `dbbench.exe` sozinho:
10 270 208 bytes (v1.61.0) e 9 871 360 bytes (v1.44.3).

## (g) Custo real de CBR — GO COM AJUSTE

`cmd/cbrbench` com `rardecode/v2 v2.4.1` (a mesma versão do módulo raiz; o `go mod tidy`
tinha puxado v2.4.5 e foi fixado de volta) e o `archive.Open` real do módulo raiz
(importado via `replace`). Amostras: os 5 `.cbr` em `C:\Users\mfdin\Downloads`
(`HQREADER_TEST_CBR` não estava definida; `spikes/rar-cost/testdata` não existe).

| Arquivo | Bytes | Assinatura | Entradas | Sólido | ComicInfo |
|---|---|---|---|---|---|
| Power Rangers – Mal Necessário | 21 074 456 | **ZIP** | 17 | — | não |
| Power Rangers – Ranger Solar | 16 065 158 | **ZIP** | 12 | — | não |
| Power Rangers – Rangers Sombrios | 21 400 087 | **ZIP** | 17 | — | não |
| Power Rangers – Rede Despedaçada #01 | 13 531 828 | RAR4 | 13 | **não** (0/13) | não |
| Os Fabulosos X-Men 175 | 30 195 907 | RAR4 | 39 | **não** (0/39) | não |

RAR (A = só cabeçalhos com `Next()`, guardando só `ComicInfo.xml`; B = `archive.Open`
+ `Pages()` + `Metadata()` + `Close()`; C = ler todos os corpos para `io.Discard`):

```
Rede Despedaçada: A header-walk (1st) entries=13 solidEntries=0 total=2.45ms | A avg over 20: 1.301ms
  C decode-all: 206ms / 207ms / 218ms      B archive.Open: 244ms / 231ms / 244ms (open 239/228/235ms)
X-Men 175:        A header-walk (1st) entries=39 solidEntries=0 total=1.605ms | A avg over 20: 1.592ms
  C decode-all: 465ms / 560ms / 532ms      B archive.Open: 559ms / 644ms / 851ms (open 543/628/835ms)
```

ZIP com extensão `.cbr`:

```
B archive.Open(as named): pages=0 ... err=archive: arquivo corrompido ou truncado
D archive/zip listing: entries=17 images=17 avg over 20=106µs err=<nil>
run1 E archive.Open(same bytes renamed .cbz): pages=17 metadata=false ... err=<nil>
```

- RAR **não sólido**: percorrer cabeçalhos custa ~1,3–1,6 ms contra 0,23–0,85 s da
  extração completa (~180–530× menos) e não grava nada em disco (o código só
  lê; não medido à parte). A Decisão 1 do plano se confirma.
- RAR **sólido: não medido** — nenhuma das amostras é sólida e não há `rar`
  nesta máquina para criar uma (só 7-Zip, que não escreve RAR). Pela fonte do
  rardecode v2.4.1, pular uma entrada num arquivo sólido decodifica o corpo
  (`decode_reader.go`, `nextFile`: `if d.solid { io.Copy(io.Discard, d) }`), então
  o custo do header-walk num sólido fica perto da coluna C: 206–560 ms nestas
  amostras (5–40 % abaixo do `archive.Open` nas mesmas rodadas, só sem gravar
  em disco). O
  `ComicInfo.xml` no fim de um sólido custa o arquivo inteiro.
- Ajuste: farejar a assinatura (contradição 2). Com 3 de 5 arquivos reais assim,
  é o caso comum desta coleção.

## (h) `NormalizePath` — GO (Decisão 9 como está)

Implementação literal da Decisão 9 (`pathnorm/pathnorm.go`):

```go
func NormalizePath(p string) string {
    abs, err := filepath.Abs(p)
    if err != nil { abs = p }
    c := strings.TrimRight(filepath.Clean(abs), `\/`)
    if runtime.GOOS == "windows" {
        c = strings.ToLower(strings.ReplaceAll(c, `\`, "/"))
    }
    return c
}
func PathID(p string) string { sum := sha256.Sum256([]byte(NormalizePath(p))); return hex.EncodeToString(sum[:])[:32] }
```

`go test ./pathnorm -v` (todos PASS):

```
"C:\\Comics\\X"        -> norm="c:/comics/x" id=ac7406680dcf14252e2bb078a76bc755 same=true
"c:/comics/x/"         -> norm="c:/comics/x" id=ac7406680dcf14252e2bb078a76bc755 same=true
"C:\\Comics\\.\\X"     -> norm="c:/comics/x" id=ac7406680dcf14252e2bb078a76bc755 same=true
"C:\\Comics\\Y\\..\\X" -> norm="c:/comics/x" ... same=true
"C:/COMICS/X//"        -> norm="c:/comics/x" ... same=true
"C:\\Comics\\Y"        -> norm="c:/comics/y" id=fedb9dcb2973c8ff37d90f361f47b54a same=false
"C:\\Comics\\Ação Especial" -> "c:/comics/ação especial" id=72b71d06a97f440ca8e78990340b8c45
"c:/comics/AÇÃO ESPECIAL/"  -> "c:/comics/ação especial" id=72b71d06a97f440ca8e78990340b8c45
stat with other case: err=<nil> isDir=true; sameID=true
mkdir NFC err=<nil>, mkdir NFD err=<nil>, dir entries=["Ação Especial" "Ão" "Ão"], sameID(NFC,NFD)=false
len(path)=340 ... len(file)=350 Stat err=<nil> size=136
zip.OpenReader ok, entries=1
WalkDir err=<nil> files=1 (len of found=350)
NormalizePath(long) len=350 stable=true id=e11ba8415235cc62736d28b9f49bf8bb
long path with \\?\ prefix same id? false
"C:\\" -> "c:"   "C:" -> "c:"   "c:/" -> "c:"   (mesmo id)
"\\\\server\\share\\Comics" -> "//server/share/comics"  (= "//server/share/comics/")
"\\\\?\\C:\\Comics\\X" -> "//?/c:/comics/x"
```

- Os quatro casos do brief se comportam como esperado; acentos e caixa
  diferente colapsam, inclusive numa pasta real.
- NFC e NFD são **pastas diferentes** no NTFS (as duas coexistiram) e recebem ids
  diferentes: correto, não normalizar Unicode.
- Caminho de 350 caracteres sob `t.TempDir()`: `MkdirAll`, `Create`, `Stat`,
  `zip.OpenReader`, `WalkDir` e `RemoveAll` funcionaram. **Ressalva:** nesta máquina
  `LongPathsEnabled = 0x1` no registro, então só o caminho com o opt-in do SO foi
  observado. Sem ele, a fonte do Go 1.27.1 (`os/path_windows.go:100`,
  `fixLongPath`) acrescenta `\\?\` sozinha — não verificado aqui.
- Limites anotados (sem ajuste necessário): raiz de unidade vira `c:` (ao montar
  `IsWithin` concatenar `/`, então `c:` + `/` funciona); um caminho com prefixo
  `\\?\` daria outro id (nenhuma fonte do app produz esse prefixo — o diálogo
  devolveu caminhos normais); caminho relativo resolve pelo diretório atual.

## O que foi rodado

`wails3 generate bindings`, `npm ci` + `npm run build` + `tsc` 5, `wails3 build DEV=true`,
`wails3 dev` (2ª tentativa; a 1ª caiu no timeout de 5 s do servidor do Vite),
`verify-ui.ps1 -Cancel` (clique real) e sem flags, `wails3 build` (produção) +
`verify-ui.ps1 -Select` e sem flags, `cmd/dbbench` (v1.61.0 duas vezes, v1.44.3 uma),
`cmd/cbrbench` (duas vezes; a 1ª com rardecode v2.4.5 e resolução de tempo ruim, descartada),
`go test ./pathnorm -v`.
