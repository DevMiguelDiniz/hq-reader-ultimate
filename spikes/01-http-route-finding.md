# Spike 1 — Rota HTTP custom no AssetServer (Wails v3)

**Versão do Wails v3 testada:** v3.0.0-beta.23 (CLI `wails3 version` e módulo Go
`github.com/wailsapp/wails/v3 v3.0.0-beta.23`, pinado em `go.mod`).

**Resultado observado:** Ao rodar `wails3 dev` e clicar no link "testar rota
custom" (`<a id="spike-link" href="/page/test-123">`) dentro da janela nativa
do app, a WebView2 navegou para a rota customizada e renderizou o texto puro:

```
spike ok: /page/test-123
```

exatamente como esperado (sem SPA, sem 404). Confirmado por duas evidências
independentes capturadas na janela real do app (não via curl/HTTP client
externo):

1. Screenshot da janela nativa (`hwnd` do processo `wails-poc.exe`, obtido via
   `Get-Process`) tirado com `System.Drawing.Graphics.CopyFromScreen` logo
   após o clique — mostra literalmente o texto `spike ok: /page/test-123`
   ocupando a página (view de texto puro padrão do WebView2/Edge para
   respostas `Content-Type: text/plain`).
2. Leitura da árvore de UI Automation (`System.Windows.Automation`) da mesma
   janela após o clique — um nó `ControlType.Text` com `Name = "spike ok:
   /page/test-123"` foi encontrado no documento carregado.

O clique em si também foi disparado dentro da janela real via UI Automation
(`AutomationElement` do hyperlink "testar rota custom" localizado na árvore de
acessibilidade da WebView2, invocado com `InvokePattern.Invoke()`), não por
uma requisição HTTP manual — ou seja, o fluxo completo (clique → navegação →
resposta do handler Go) foi exercitado dentro do WebView gerenciado pelo
Wails.

Observação técnica relevante: em modo dev, o log do processo mostrou
`AssetServer Info: middleware=true handler=true devServerURL=http://localhost:9245`.
O Wails detecta o Vite dev server (`FRONTEND_DEVSERVER_URL`) e usa
`application.AssetFileServerFS` para fazer proxy de `/` para ele, mas isso não
interferiu na rota `/page/`: como o roteamento é feito por um
`http.ServeMux` próprio *antes* de qualquer chamada a `AssetFileServerFS`, a
rota customizada é resolvida diretamente pelo nosso handler tanto em dev
quanto em build de produção. Também vale registrar que no Windows a base URL
do AssetServer é `http://wails.localhost` (mapeada internamente pelo
WebView2, não um listener TCP acessível de fora do processo) — por isso a
verificação teve que ser feita dentro da janela real do app, e não com
`curl`/requisição HTTP direta a uma porta.

**Nome real do campo/função usado:** `application.AssetOptions.Handler`
(`http.Handler`) e `application.AssetFileServerFS(assets fs.FS) http.Handler`
— **idênticos** aos nomes assumidos no brief (`Assets.Handler` e
`AssetFileServerFS`). Confirmado via:

```
go doc github.com/wailsapp/wails/v3/pkg/application AssetOptions
go doc github.com/wailsapp/wails/v3/pkg/application AssetFileServerFS
```

Nenhum ajuste de nome de API foi necessário nesta versão (v3.0.0-beta.23).

**Decisão:** **GO** — registrar um `http.Handler` customizado (mux próprio
com uma rota `/page/...` e fallback para `AssetFileServerFS`) em
`AssetOptions.Handler` funciona de ponta a ponta, tanto em `wails3 dev`
quanto em build de produção (confirmado por medição, ver seção seguinte — não
apenas inferido do código). Isso valida a aposta arquitetural central do
design: servir páginas de quadrinhos via rota HTTP customizada ao lado do
AssetServer, sem depender de IPC/bindings.

## Confirmação adicional: build de produção (fix wave pós-revisão final)

A verificação original acima (screenshot + UI Automation) só exercitou a
rota `/page/` via `wails3 dev`, onde o `AssetServer` faz proxy de `/` para o
Vite dev server. A frase "seria esperado" no parágrafo de decisão original
descrevia uma inferência (mesmo `Options.Handler` usado em ambos os modos),
não uma medição — a aposta arquitetural central do design nunca tinha sido
testada no caminho real de produção (assets embutidos via `embed.FS`, sem
Vite, sem proxy).

Isso foi fechado rodando o binário de produção real, gerado por
`wails3 build` (`spikes/wails-poc/bin/wails-poc.exe`, `go build -tags
production ...`, sem servidor de dev envolvido), e repetindo a mesma técnica
de verificação da Task 2 (Windows UI Automation dentro da janela nativa real,
não curl/HTTP externo):

1. `cd spikes/wails-poc && wails3 build` — build de produção limpo, sucesso.
2. `spikes/01-http-route-prod-verify.ps1` (script comitado, reexecutável)
   inicia `bin/wails-poc.exe` diretamente (sem `wails3 dev`), localiza a
   janela nativa real via UI Automation, invoca o link "testar rota custom"
   (`InvokePattern.Invoke()`) e lê de volta o nó de texto renderizado.
3. Saída bruta real (comitada em `spikes/01-http-route-prod-verify-output.txt`):

```
=== Launching PRODUCTION binary: ...\spikes\wails-poc\bin\wails-poc.exe ===
PID: 22476
Found window: Window 1

=== Locating 'testar rota custom' link ===
Link found. Invoking via InvokePattern (production embedded-assets path, no Vite dev server involved).

=== Reading rendered text after navigation ===
PRODUCTION ROUTE OK: found text node 'spike ok: /page/test-123'

=== Cleanup ===
Killed PID 22476.

=== DONE ===
```

**Conclusão:** a rota `/page/` funciona idêntica em produção e em dev —
`spike ok: /page/test-123` foi renderizado corretamente no binário embutido
(`embed.FS`, sem Vite), fechando a lacuna de verificação apontada pela
revisão final de branch. Nenhum ajuste de arquitetura necessário; a decisão
GO permanece, agora com evidência de medição nos dois modos.

**Se NO-GO, alternativa a investigar:** N/A (spike terminou GO). Caso
surjam problemas futuros de precedência de rotas com o `AssetOptions.Middleware`
(que roda *antes* de qualquer handler, inclusive o nosso mux), essa é a
extensão natural a explorar para casos como range requests / cache-control
customizados por página.
