# Etapa 0 — Spikes de Validação de Risco — Plano de Implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Responder, com evidência executável, os 5 riscos go/no-go do §5 do
design antes de escrever qualquer código de produto.

**Architecture:** Cada risco vira um PoC isolado e descartável dentro de
`spikes/`. Dois riscos (rota HTTP custom e multi-window) compartilham um único
app Wails v3 mínimo gerado no Task 1. Os outros três (custo de RAR sólido,
pan/zoom WebKitGTK, exigência de CGO no Windows) são independentes. A etapa
termina consolidando os 5 achados numa decisão go/no-go única.

**Tech Stack:** Go 1.21+, Wails v3 (CLI `wails3`), `github.com/nwaples/rardecode/v2`.

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — §5 (Riscos e validação).

## Global Constraints

- Wails v3 está em beta: depois de instalado, registrar a versão exata
  resolvida em `go.mod`/`go.sum` de `spikes/wails-poc` e não trocar para
  `@latest` durante a etapa — trocar de versão no meio de um spike invalida a
  medição.
- Go 1.21+ é exigido pelo Wails v3.
- Tudo em `spikes/` é código descartável — não precisa seguir os mesmos
  padrões de qualidade/estrutura do produto final (Etapa 1 em diante).
- Nenhuma amostra de CBR ou imagem grande usada nos spikes é commitada ao
  git — vai para `.gitignore`.
- Cada spike termina com um arquivo de achado em Markdown (`spikes/0N-*.md`)
  contendo: o que foi testado, o resultado observado, e uma decisão explícita
  de go/no-go (ou "requer mitigação: <qual>").

---

### Task 1: Ambiente Wails v3 mínimo (base para Tasks 2 e 4)

**Files:**
- Create: `spikes/README.md`
- Create: `spikes/wails-poc/` (gerado pelo CLI `wails3`)
- Modify: `.gitignore` (adicionar exclusões de amostras grandes)

**Interfaces:**
- Produces: um app Wails v3 rodável em `spikes/wails-poc/` que as Tasks 2 e 4
  vão modificar.

- [ ] **Step 1: Confirmar toolchain**

Rodar:
```bash
go version
```
Esperado: versão 1.21 ou superior. Se for menor, atualizar o Go antes de continuar.

- [ ] **Step 2: Instalar a CLI do Wails v3**

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 version
```
Anotar a versão exata impressa — ela é a versão "pinada" para toda a Etapa 0.

- [ ] **Step 3: Rodar o diagnóstico de dependências da plataforma**

```bash
wails3 doctor
```
Resolver qualquer dependência de sistema reportada como faltante (WebView2
Runtime no Windows; `gtk3`/`webkitgtk` ou `libwebkitgtk-6.0` no Linux;
Xcode command line tools no macOS). Não prosseguir com pendência vermelha.

- [ ] **Step 4: Criar `spikes/README.md`**

```markdown
# spikes/

Código descartável usado para validar os riscos do §5 de
`docs/superpowers/specs/2026-09-16-hq-reader-design.md` antes de iniciar o
produto. Nada aqui segue os padrões de qualidade do código de produção.
```

- [ ] **Step 5: Gerar o app mínimo**

```bash
cd spikes
wails3 init -n wails-poc -t vanilla
```

Se a flag `-t vanilla` não existir na versão instalada, rodar
`wails3 init -help` para achar o nome atual do template mínimo (JS/HTML puro,
sem framework de UI) e usar esse.

- [ ] **Step 6: Confirmar que o template roda**

```bash
cd spikes/wails-poc
wails3 dev
```
Esperado: uma janela nativa abre mostrando a página padrão do template. Parar
o processo (Ctrl+C) depois de confirmar.

- [ ] **Step 7: Atualizar `.gitignore` e commitar o scaffold**

Adicionar ao `.gitignore` na raiz do repo:
```
spikes/**/testdata/
spikes/**/*.cbr
spikes/**/*.cbz
spikes/wails-poc/frontend/sample-*.jpg
```

```bash
git add spikes/README.md spikes/wails-poc .gitignore
git commit -m "spike: scaffold app Wails v3 minimo para etapa 0"
```

---

### Task 2: Spike — rota HTTP custom no AssetServer (§5 item 1)

**Files:**
- Modify: `spikes/wails-poc/main.go`
- Modify: `spikes/wails-poc/frontend/index.html` (ou o caminho onde o template
  do Task 1 colocou a página inicial — confirmar o caminho real gerado antes
  de editar)
- Create: `spikes/01-http-route-finding.md`

**Interfaces:**
- Consumes: o app gerado na Task 1.
- Produces: achado registrado em `spikes/01-http-route-finding.md`, usado na
  Task 7.

- [ ] **Step 1: Verificar o formato de `application.Options` instalado**

```bash
cd spikes/wails-poc
go doc github.com/wailsapp/wails/v3/pkg/application Options
go doc github.com/wailsapp/wails/v3/pkg/application AssetOptions
```
Conferir se existe um campo para plugar um `http.Handler` customizado (ex.:
`Assets.Handler`). Anotar o nome exato do campo encontrado — os steps
seguintes assumem `Assets.Handler`; se o nome for diferente, ajustar o código
abaixo para o nome real.

- [ ] **Step 2: Escrever o handler de teste e o mux combinado**

Editar `spikes/wails-poc/main.go`:

```go
package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/page/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "spike ok: %s", r.URL.Path)
	})

	mux.Handle("/", application.AssetFileServerFS(assets))

	app := application.New(application.Options{
		Name: "wails-poc",
		Assets: application.AssetOptions{
			Handler: mux,
		},
	})

	app.NewWebviewWindow()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

Se `application.AssetFileServerFS` não existir com esse nome, rodar
`go doc github.com/wailsapp/wails/v3/pkg/application` (sem filtro) e procurar
a função equivalente de servir o `embed.FS` do frontend — substituir a
chamada mantendo o resto da estrutura.

- [ ] **Step 3: Adicionar um link de teste na página inicial**

No arquivo HTML gerado pelo Task 1 (dentro de `spikes/wails-poc/frontend/`),
adicionar antes do fechamento de `</body>`:

```html
<a id="spike-link" href="/page/test-123">testar rota custom</a>
```

- [ ] **Step 4: Rodar e verificar manualmente**

```bash
wails3 dev
```
Clicar no link "testar rota custom" dentro da janela do app. Esperado: a
página navega e mostra o texto puro `spike ok: /page/test-123` — não a SPA,
não um 404.

- [ ] **Step 5: Registrar o achado**

Criar `spikes/01-http-route-finding.md`:

```markdown
# Spike 1 — Rota HTTP custom no AssetServer (Wails v3)

**Versão do Wails v3 testada:** <preencher com o output de `wails3 version`>

**Resultado observado:** <colar o texto exibido ao clicar no link, ou o erro
obtido>

**Nome real do campo/função usado:** <Assets.Handler / outro nome encontrado
no Step 1>

**Decisão:** GO / NO-GO — <justificativa em 1-2 frases>

**Se NO-GO, alternativa a investigar:** <ex.: middleware separado, servidor
HTTP próprio em porta distinta, etc.>
```

- [ ] **Step 6: Commit**

```bash
git add spikes/wails-poc spikes/01-http-route-finding.md
git commit -m "spike: valida rota HTTP custom no AssetServer do Wails v3"
```

---

### Task 3: Spike — custo de extração de RAR sólido (§5 item 2)

**Files:**
- Create: `spikes/rar-cost/main.go`
- Create: `spikes/rar-cost/go.mod` (módulo Go independente)
- Create: `spikes/02-rar-cost-finding.md`

**Interfaces:**
- Consumes: nenhuma dependência das outras tasks.
- Produces: achado registrado em `spikes/02-rar-cost-finding.md`.

- [ ] **Step 1: Inicializar o módulo e a dependência**

```bash
mkdir -p spikes/rar-cost
cd spikes/rar-cost
go mod init hqreader/spikes/rarcost
go get github.com/nwaples/rardecode/v2
```

- [ ] **Step 2: Escrever o programa de medição**

Criar `spikes/rar-cost/main.go`:

```go
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nwaples/rardecode/v2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("uso: rar-cost <arquivo.cbr>")
		os.Exit(1)
	}
	path := os.Args[1]

	start := time.Now()
	r, err := rardecode.OpenReader(path, "")
	if err != nil {
		fmt.Printf("erro ao abrir %s: %v\n", path, err)
		os.Exit(1)
	}
	defer r.Close()

	entryCount := 0
	var firstEntryAt time.Duration

	for {
		hdr, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("erro lendo cabecalho da entrada %d: %v\n", entryCount, err)
			os.Exit(1)
		}

		if _, err := io.Copy(io.Discard, r); err != nil {
			fmt.Printf("erro decodificando entrada %d (%s): %v\n", entryCount, hdr.Name, err)
			os.Exit(1)
		}

		entryCount++
		if entryCount == 1 {
			firstEntryAt = time.Since(start)
		}
	}

	total := time.Since(start)
	fmt.Printf("arquivo: %s\n", path)
	fmt.Printf("entradas decodificadas: %d\n", entryCount)
	fmt.Printf("tempo ate a 1a entrada: %v\n", firstEntryAt)
	fmt.Printf("tempo total (todas as entradas ate o fim): %v\n", total)
}
```

- [ ] **Step 3: Rodar sem amostra pra confirmar o erro de uso**

```bash
go run . 
```
Esperado: imprime `uso: rar-cost <arquivo.cbr>` e sai com código 1 (confirma
que o binário compila e a validação de argumento funciona antes de testar com
arquivo real).

- [ ] **Step 4: Rodar contra uma amostra real de CBR sólido**

Colocar um `.cbr` real conhecido como sólido (ex.: exportado do WinRAR com a
opção "Solid archive" marcada, ou uma release de scan que tipicamente usa
compressão sólida) em `spikes/rar-cost/testdata/sample-solid.cbr` (não
commitado — já coberto pelo `.gitignore` da Task 1).

```bash
go run . testdata/sample-solid.cbr
```
Anotar a saída completa.

- [ ] **Step 5: Rodar contra uma amostra CBR não-sólida para comparação**

Repetir o Step 4 com um `.cbr` comum (compressão não-sólida) em
`testdata/sample-normal.cbr`. Anotar a saída.

- [ ] **Step 6: Registrar o achado**

Criar `spikes/02-rar-cost-finding.md`:

```markdown
# Spike 2 — Custo de extração de RAR sólido

**Amostra sólida:** <nome, tamanho em MB, numero de paginas>
- tempo ate a 1a entrada: <valor>
- tempo total: <valor>

**Amostra não-sólida (comparação):**
- tempo ate a 1a entrada: <valor>
- tempo total: <valor>

**Decisão:** GO / NO-GO / GO COM MITIGAÇÃO — <justificativa>

**Se precisar de mitigação:** <ex.: indicador de progresso na abertura do
livro na Etapa 3, definido como obrigatório se tempo total > X segundos>
```

- [ ] **Step 7: Commit**

```bash
git add spikes/rar-cost spikes/02-rar-cost-finding.md
git commit -m "spike: mede custo de extracao de RAR solido"
```

---

### Task 4: Spike — pan/zoom no WebKitGTK (Linux) com imagem 3000px (§5 item 3)

**Files:**
- Modify: `spikes/wails-poc/frontend/index.html`
- Create: `spikes/03-pan-zoom-finding.md`

**Interfaces:**
- Consumes: o app gerado na Task 1 (pode reusar o mesmo `main.go` da Task 2,
  já que a rota `/page/` não interfere neste spike).

- [ ] **Step 1: Obter uma imagem de amostra**

Colocar um JPEG real de aproximadamente 3000px de largura (uma página de HQ
escaneada, por exemplo) em
`spikes/wails-poc/frontend/sample-3000px.jpg` (já coberto pelo `.gitignore`
da Task 1).

- [ ] **Step 2: Substituir o conteúdo da página por um visor com pan/zoom**

Editar `spikes/wails-poc/frontend/index.html`, substituindo o `<body>` pelo
seguinte:

```html
<style>
  html, body { margin: 0; height: 100%; background: #222; }
  #viewport {
    overflow: hidden;
    width: 100vw;
    height: 100vh;
    touch-action: none;
  }
  #page {
    transform-origin: 0 0;
    will-change: transform;
    user-select: none;
  }
</style>
<div id="viewport">
  <img id="page" src="sample-3000px.jpg" draggable="false" />
</div>
<script>
  const img = document.getElementById('page');
  let scale = 1, x = 0, y = 0, dragging = false, lastX = 0, lastY = 0;

  function apply() {
    img.style.transform = `translate(${x}px, ${y}px) scale(${scale})`;
  }

  window.addEventListener('wheel', (e) => {
    e.preventDefault();
    const delta = e.deltaY > 0 ? 0.9 : 1.1;
    scale = Math.min(6, Math.max(0.2, scale * delta));
    apply();
  }, { passive: false });

  window.addEventListener('pointerdown', (e) => {
    dragging = true;
    lastX = e.clientX;
    lastY = e.clientY;
  });
  window.addEventListener('pointermove', (e) => {
    if (!dragging) return;
    x += e.clientX - lastX;
    y += e.clientY - lastY;
    lastX = e.clientX;
    lastY = e.clientY;
    apply();
  });
  window.addEventListener('pointerup', () => { dragging = false; });
</script>
```

- [ ] **Step 3: Rodar numa máquina/VM Linux real com WebKitGTK**

```bash
wails3 dev
```
Este spike só responde à pergunta do §5 se rodado no Linux — rodar em
Windows/macOS aqui não valida nada, porque usam WebView2/WKWebView, não
WebKitGTK.

- [ ] **Step 4: Testar manualmente e observar fluidez**

Usar a roda do mouse sobre a imagem para dar zoom, e arrastar (clique +
movimento) para dar pan. Observar a olho nu se o movimento é fluido ou
engasga perceptivelmente, especialmente em zoom alto (> 3x).

- [ ] **Step 5: Registrar o achado**

Criar `spikes/03-pan-zoom-finding.md`:

```markdown
# Spike 3 — Pan/zoom em WebKitGTK com imagem 3000px

**Distribuição/versão do WebKitGTK testada:** <preencher>

**Observação:** fluido / engasga em zoom baixo / engasga em zoom alto /
engasga sempre — <detalhar>

**Decisão:** GO / NO-GO / GO COM MITIGAÇÃO

**Se precisar de mitigação:** <ex.: downscale da imagem para exibição em telas
menores, `will-change` adicional, renderização por canvas em vez de `<img>`>
```

- [ ] **Step 6: Commit**

```bash
git add spikes/wails-poc/frontend/index.html spikes/03-pan-zoom-finding.md
git commit -m "spike: testa pan/zoom no WebKitGTK com imagem grande"
```

---

### Task 5: Spike — multi-window compartilhando estado (§5 item 4)

**Files:**
- Modify: `spikes/wails-poc/main.go`
- Modify: `spikes/wails-poc/frontend/index.html`
- Create: `spikes/04-multiwindow-finding.md`

**Interfaces:**
- Consumes: o app gerado na Task 1.
- Produces: achado registrado em `spikes/04-multiwindow-finding.md`.

- [ ] **Step 1: Verificar a API de registro de serviços/bindings da versão instalada**

```bash
cd spikes/wails-poc
go doc github.com/wailsapp/wails/v3/pkg/application Options
```
Confirmar o nome do campo usado para expor um service ao frontend (ex.:
`Services`, `Bind`) e a assinatura esperada (ex.: `application.NewService(x)`
vs. passar o valor direto). Ajustar o código do Step 2 para o nome real
encontrado.

- [ ] **Step 2: Implementar um service compartilhado simples**

Editar `spikes/wails-poc/main.go`:

```go
package main

import (
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type CounterService struct {
	mu    sync.Mutex
	count int
}

func (c *CounterService) Increment() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	return c.count
}

func (c *CounterService) Get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func main() {
	counter := &CounterService{}

	app := application.New(application.Options{
		Name: "wails-poc",
		Services: []application.Service{
			application.NewService(counter),
		},
	})

	win1 := app.NewWebviewWindow()
	win1.SetTitle("Janela 1")

	win2 := app.NewWebviewWindow()
	win2.SetTitle("Janela 2")

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 3: Gerar os bindings e ligar a UI**

Rodar `wails3 dev` uma vez para o Wails gerar os bindings TypeScript/JS do
`CounterService` (tipicamente em `spikes/wails-poc/frontend/bindings/`).
Verificar o caminho e a assinatura exata gerada, então editar
`spikes/wails-poc/frontend/index.html` importando e chamando essa assinatura
gerada (o nome exato do import depende do binding gerado — usar o que
aparecer no diretório de bindings, não adivinhar):

```html
<button id="inc">incrementar</button>
<div id="value">0</div>
<script type="module">
  // ajustar o caminho de import conforme o binding gerado em frontend/bindings/
  import { Increment, Get } from './bindings/main/counterservice.js';

  const valueEl = document.getElementById('value');

  async function refresh() {
    valueEl.textContent = await Get();
  }

  document.getElementById('inc').addEventListener('click', async () => {
    await Increment();
    await refresh();
  });

  refresh();
</script>
```

- [ ] **Step 4: Testar compartilhamento de estado entre janelas**

Com `wails3 dev` rodando: clicar "incrementar" várias vezes na Janela 1.
Trocar para a Janela 2, recarregar (ou clicar em algum botão que chame
`Get()`) e confirmar que o valor mostrado é o mesmo contador, refletindo os
cliques feitos na Janela 1.

- [ ] **Step 5: Testar fechar uma janela sem matar o app**

Fechar a Janela 2 (botão de fechar do SO). Esperado: a Janela 1 continua
funcional e o processo do app segue rodando (confirma que fechar uma janela
não derruba o estado do backend nem o processo inteiro — pré-requisito para a
Etapa 7 de destacar/reacoplar).

- [ ] **Step 6: Registrar o achado**

Criar `spikes/04-multiwindow-finding.md`:

```markdown
# Spike 4 — Multi-window compartilhando estado (Wails v3)

**Nome real do campo de registro de service encontrado:** <preencher>

**Estado compartilhado entre janelas:** sim / não — <detalhar o que foi
observado>

**Fechar uma janela mata o processo?** sim / não

**Decisão:** GO / NO-GO — <justificativa>

**Se NO-GO, alternativa a investigar:** <ex.: sincronizar via eventos em vez
de estado compartilhado direto>
```

- [ ] **Step 7: Commit**

```bash
git add spikes/wails-poc spikes/04-multiwindow-finding.md
git commit -m "spike: valida estado compartilhado entre janelas no Wails v3"
```

---

### Task 6: Spike — exigência de CGO no build Windows (§5 item 5)

**Files:**
- Create: `spikes/05-windows-cgo-finding.md`

**Interfaces:**
- Consumes: o app gerado na Task 1 (usa o `spikes/wails-poc` já existente,
  sem precisar das modificações das Tasks 2/4/5 — pode rodar a partir do
  estado deixado pela Task 1 sozinha, num checkout limpo se necessário).

- [ ] **Step 1: Build com CGO desabilitado (Windows, PowerShell)**

```powershell
cd spikes/wails-poc
$env:CGO_ENABLED = "0"
wails3 build
```
Anotar se o build falha (e a mensagem de erro completa, se falhar) ou
completa com sucesso.

- [ ] **Step 2: Build com CGO habilitado, para comparação**

```powershell
$env:CGO_ENABLED = "1"
wails3 build
```
Confirmar que este completa (baseline de que o ambiente de build está
corretamente configurado com um compilador C, ex. `gcc` via MSYS2/TDM-GCC).

- [ ] **Step 3: Registrar o achado**

Criar `spikes/05-windows-cgo-finding.md`:

```markdown
# Spike 5 — Exigência de CGO no build Windows

**Build com CGO_ENABLED=0:** sucesso / falha — <mensagem de erro, se houver>

**Build com CGO_ENABLED=1:** sucesso / falha

**Conclusão:** o Wails v3 no Windows exige / não exige CGO.

**Impacto na escolha do driver SQLite (`mattn/go-sqlite3`, que exige CGO):**
<se Windows já exige CGO por causa do próprio Wails, a escolha não adiciona
custo novo. Se não exigir, registrar que a escolha do driver introduz uma
exigência de toolchain C que não existiria de outra forma, e se isso muda a
decisão do §4.1 do design.>
```

- [ ] **Step 4: Commit**

```bash
git add spikes/05-windows-cgo-finding.md
git commit -m "spike: confirma exigencia de CGO no build Windows do Wails v3"
```

---

### Task 7: Consolidação e decisão go/no-go da Etapa 0

**Files:**
- Create: `spikes/FINDINGS.md`
- Modify: `docs/superpowers/specs/2026-09-16-hq-reader-design.md` (§5)

**Interfaces:**
- Consumes: os 5 arquivos de achado criados nas Tasks 2-6.
- Produces: `spikes/FINDINGS.md`, referenciado pelo spec atualizado.

- [ ] **Step 1: Escrever o resumo consolidado**

Criar `spikes/FINDINGS.md` copiando a decisão (GO/NO-GO/GO COM MITIGAÇÃO) e a
justificativa de cada um dos 5 arquivos de achado das Tasks 2-6, em uma
tabela única:

```markdown
# Etapa 0 — Achados consolidados

| # | Risco | Decisão | Nota |
|---|---|---|---|
| 1 | Rota HTTP custom no AssetServer | <preencher> | <preencher> |
| 2 | Custo de extração de RAR sólido | <preencher> | <preencher> |
| 3 | Pan/zoom WebKitGTK 3000px | <preencher> | <preencher> |
| 4 | Multi-window com estado compartilhado | <preencher> | <preencher> |
| 5 | Exigência de CGO no Windows | <preencher> | <preencher> |

Detalhes de cada spike: ver `spikes/0N-*-finding.md`.
```

- [ ] **Step 2: Atualizar o spec com o resultado**

Editar `docs/superpowers/specs/2026-09-16-hq-reader-design.md`, na seção §5,
adicionando ao final da seção (sem apagar a lista original de riscos):

```markdown
**Status:** validado na Etapa 0. Ver `spikes/FINDINGS.md` para o resultado
detalhado de cada item e eventuais ajustes de arquitetura decorrentes.
```

Se algum item saiu como NO-GO ou GO COM MITIGAÇÃO, editar também a seção do
spec correspondente (§3.1 para a rota HTTP, §3.3 para o custo de RAR, §4.1
para o driver SQLite) registrando a mitigação decidida — não deixar a
mitigação só no `FINDINGS.md`, o spec precisa refletir a arquitetura real que
vai pra Etapa 1 em diante.

- [ ] **Step 3: Commit final da Etapa 0**

```bash
git add spikes/FINDINGS.md docs/superpowers/specs/2026-09-16-hq-reader-design.md
git commit -m "spike: consolida achados da etapa 0 e atualiza o spec"
```

---

## Self-Review

**Cobertura do spec:** os 5 itens do §5 do design têm uma task dedicada cada
(Tasks 2-6); Task 1 é infraestrutura compartilhada; Task 7 fecha o ciclo
atualizando o próprio spec, o que também cobre a exigência do design de que a
Etapa 0 produza "relatório curto com o resultado de cada item" (§6 do design).

**Placeholders:** os únicos `<preencher>` que restam são os campos de dado
observado (tempos medidos, versões instaladas, mensagens de erro reais) que
só existem depois de rodar os spikes — não são lacunas de plano, são os
resultados do experimento em si.

**Consistência de tipos/nomes:** `CounterService.Increment()`/`Get()` na Task
5 são usados com o mesmo nome no Go e no JS de exemplo; `spikes/wails-poc`
como caminho do scaffold é usado de forma consistente nas Tasks 2, 4, 5 e 6.
