# Spike 4 — Multi-window compartilhando estado (Wails v3)

**Versão do Wails v3 testada:** v3.0.0-beta.23 (pinada em `go.mod`).

**Nome real do campo de registro de service encontrado:** `application.Options.Services []application.Service`,
populado com `application.NewService[T any](instance *T) Service` — **idêntico**
ao assumido no brief. Confirmado via:

```
go doc github.com/wailsapp/wails/v3/pkg/application Options
go doc github.com/wailsapp/wails/v3/pkg/application NewService
go doc github.com/wailsapp/wails/v3/pkg/application Service
```

Um ponto de API **divergiu** do brief: não existe `app.NewWebviewWindow()`.
A criação de janelas é feita via o gerenciador `app.Window` (`*application.WindowManager`,
campo `Window` de `*application.App`), com `app.Window.NewWithOptions(application.WebviewWindowOptions{...})`
(ou `app.Window.New()` para opções default) — confirmado via:

```
go doc github.com/wailsapp/wails/v3/pkg/application WindowManager
```

Isso já era o padrão usado no `main.go` existente (herdado do scaffold da Task 1
para a primeira janela), então a segunda janela deste spike apenas replicou o
mesmo padrão (`app.Window.NewWithOptions(...)` com `Title: "Window 2"`).

Os bindings TS gerados por `wails3 dev` (`frontend/bindings/changeme/counterservice.ts`)
confirmaram as assinaturas de `Increment()`/`Get()` exatamente como esperado
(`$Call.ByID(<hash>)` por método, retornando `CancellablePromise<number>`), e o
barrel `frontend/bindings/changeme/index.ts` passou a exportar `CounterService`
ao lado do `GreetService` já existente — o import usado em `main.ts` seguiu o
mesmo padrão já presente no scaffold: `import {GreetService, CounterService} from "../bindings/changeme"`.

**Estado compartilhado entre janelas:** **sim** — confirmado com evidência real,
não apenas inspeção de código. Com `wails3 dev` rodando (duas janelas nativas,
"Window 1" e "Window 2", abertas pelo mesmo processo `wails-poc.exe`), um script
PowerShell usando Windows UI Automation (`System.Windows.Automation`, mesma
técnica da Task 2) invocou o botão real "incrementar" (via `InvokePattern.Invoke()`)
e leu o valor real exibido (nó `ControlType.Text` cujo `Name` é o número do
contador) dentro de cada janela:

1. 3 cliques em "incrementar" na Window 1 → Window 1 passou a mostrar `3`.
2. Antes de qualquer interação na Window 2, seu valor (herdado do carregamento
   inicial, antes dos cliques na Window 1) ainda mostrava `0` — esperado, já
   que não há push automático de evento, só `Get()` sob demanda.
3. 1 clique em "incrementar" na Window 2 (que chama `Increment()` e depois
   `Get()` no mesmo backend) → Window 2 passou a mostrar `4` = 3 (feito na
   Window 1) + 1 (feito na própria Window 2) — **não** `1`, o que prova que as
   duas janelas leem/escrevem o mesmo `CounterService` (mesmo processo Go,
   mesmo ponteiro protegido por mutex), e não instâncias isoladas por janela.

**Fechar uma janela mata o processo?** **não**. Fechei a Window 2 via
`WindowPattern.Close()` na árvore de UI Automation (equivalente programático ao
botão de fechar do SO — aciona o mesmo caminho de fechamento nativo da janela,
não um `kill` de processo). Depois disso:

- `Get-Process wails-poc` continuou retornando o mesmo PID, com `HasExited = False`
  — o processo `wails-poc.exe` seguiu rodando.
- A Window 1 permaneceu presente e funcional: um clique adicional em
  "incrementar" nela fez o contador ir para `5` (= 4 + 1), confirmando que o
  `CounterService` compartilhado sobreviveu ao fechamento da Window 2 e que a
  Window 1 continuou operando normalmente (backend + IPC intactos).

**Regressão (Task 2):** o link "testar rota custom" (rota HTTP custom `/page/`
do spike da Task 2) continuou funcionando na Window 1 após todas as mudanças
desta task — clicado via UI Automation, renderizou `spike ok: /page/test-123`
como antes.

**Decisão:** **GO** — Wails v3 (beta.23) suporta criar múltiplas janelas no
mesmo processo, todas compartilhando o mesmo backend Go (mesmos services,
mesmo estado em memória), e fechar uma janela não derruba o processo nem afeta
o estado ou a funcionalidade das janelas restantes. Isso valida o
pré-requisito arquitetural da Etapa 7 (destacar/reacoplar janela mantendo
estado compartilhado): o padrão "múltiplas `WebviewWindowOptions` + um único
`Services` list" é viável tal como desenhado, sem necessidade de sincronização
manual entre processos separados.

**Se NO-GO, alternativa a investigar:** N/A (spike terminou GO). Observação
para a Etapa 7: como não há push automático de mudanças de estado entre
janelas (cada janela só vê o estado atualizado quando ela mesma chama um
método do service, ex. `Get()`), o design de "destacar janela" deve usar
eventos (`application.Event.Emit`/`Events.On`, já usados neste mesmo scaffold
para o evento `time`) para notificar outras janelas quando o estado mudar,
em vez de assumir atualização automática só pelo estado ser compartilhado no
backend.
