# Etapa 0 — Achados consolidados

**Status geral:** 4 de 5 itens concluídos. Item 3 (pan/zoom WebKitGTK) está
**PENDENTE** — não executado nesta rodada por falta de ambiente Linux
disponível nesta máquina (ver nota na linha do item).

| # | Risco | Decisão | Nota |
|---|---|---|---|
| 1 | Rota HTTP custom no AssetServer | **GO** | Registrar `http.Handler` customizado em `AssetOptions.Handler` (mux próprio com rota `/page/...` + fallback para `AssetFileServerFS`) funciona de ponta a ponta em `wails3 dev`, confirmado por screenshot + UI Automation da janela real (não via curl externo). Nomes de API (`AssetOptions.Handler`, `AssetFileServerFS`) idênticos ao brief, sem ajuste necessário na v3.0.0-beta.23. |
| 2 | Custo de extração de RAR sólido | **GO COM MITIGAÇÃO** | Única amostra real disponível (28.8 MiB / 39 entradas) levou ~537ms para decodificação sequencial completa — mas se revelou **não-sólida** (`hdr.Solid == false` em todas as entradas), então o pior caso genuíno de RAR sólido não foi medido. Extrapolação linear pelo throughput observado (~53.6 MiB/s) sugere ~1.87s para 100 MiB e ~3.73s para 200 MiB — faixa perceptível ao usuário sem feedback. Mitigação: indicador de progresso na abertura, ativado quando `hdr.Solid == true` **e** tempo estimado > limiar inicial de 300–500ms (ajustável). Lacuna: validar com amostra CBR genuinamente sólida e grande (100+ MiB) quando disponível. |
| 3 | Pan/zoom WebKitGTK 3000px | **PENDENTE** | Requer ambiente Linux com WebKitGTK real — não disponível nesta máquina. Rodar quando um ambiente Linux (VM, máquina física, ou WSL2 com distro instalada) estiver disponível, seguindo os passos da Task 4 do plano de implementação. |
| 4 | Multi-window com estado compartilhado | **GO** | Duas janelas Wails v3 no mesmo processo compartilham o mesmo `CounterService` (mesmo backend, mesmo estado em memória) — confirmado por UI Automation real (cliques e leitura de valores nas janelas nativas, não inspeção de código). Fechar uma janela (via `WindowPattern.Close()`) não mata o processo nem afeta a janela restante ou o estado compartilhado. Divergência de API do brief: não existe `app.NewWebviewWindow()`; o padrão real é `app.Window.NewWithOptions(...)` via `*application.WindowManager`. Caveat para a Etapa 7: não há push automático de estado entre janelas — cada janela só vê o estado atualizado ao chamar um método do service; um design de destacar/reacoplar janela precisa usar eventos (`Events.On`/`Emit`), não depender de estado compartilhado passivo. |
| 5 | Exigência de CGO no Windows | **GO (Wails) COM RISCO ABERTO (driver SQLite)** | Wails v3 **não exige CGO** para sua própria compilação no Windows (builds com `CGO_ENABLED=0` e `CGO_ENABLED=1` ambos tiveram sucesso, sem C compiler invocado — o binding de WebView2 usa COM/syscall, Go puro). Porém: esta máquina de desenvolvimento **não tem nenhum compilador C instalado** (gcc/cc/clang ausentes do PATH). Isso significa que manter `mattn/go-sqlite3` (driver CGO, já decidido em §4.1 do design) causaria **falha imediata de build** nesta máquina, e provavelmente em qualquer outra máquina Windows de dev/CI sem MSYS2 ou TDM-GCC pré-instalados. Este achado **não resolve** a decisão de driver por conta própria — ver nota adicionada ao spec (§4), que sinaliza a necessidade de revisão consciente pelo dono do projeto, sem alterar a escolha registrada na tabela de decisões. |

Detalhes de cada spike: ver `spikes/0N-*-finding.md` (não há `spikes/03-*`
ainda — item 3 permanece pendente, sem spike executado nesta rodada).

## Nota técnica: bug conhecido do `wails3 dev` (scaffold gerado, Taskfile)

Achado durante a Task 1 (ambiente base) e corrigido no commit `d14218a`, mas
até esta nota não estava registrado em nenhum lugar durável — só na mensagem
do commit e num relatório de task efêmero. Como tudo em `spikes/` é
descartado ao fim da Etapa 0 (`spikes/wails-poc` não vira parte do produto),
sem este registro a Etapa 2 (que gera um app Wails de produção do zero, com o
mesmo scaffold do CLI `wails3 init`) bateria no mesmo bug sem nenhuma pista do
porquê ou da correção.

**Sintoma:** `wails3 dev` falhava de forma reproduzível (3/3 execuções) com
`unable to connect to frontend server` (FATAL), poucos segundos após iniciar.

**Causa raiz:** o binário do `wails3 dev` tem um retry fixo de ~5s (não
configurável) esperando o Vite dev server responder. A task
`install:frontend:deps:npm` do `build/Taskfile.yml` gerado pelo scaffold do
CLI (`wails3 init`) tinha um bug de cópia: usava `generates: [node_modules]`
em vez de um `status` check, o que faz o Task (o executor de tasks, não
Wails) considerar a task **sempre desatualizada** — `generates` não substitui
`status` para uma checagem de "já está pronto, pula". Resultado: `npm
install` reexecutava em **toda** invocação de `wails3 dev`, mesmo com
`node_modules` já presente e populado, consumindo ~2-3s do orçamento fixo de
5s do retry — o suficiente para o Vite não subir a tempo e o `wails3 dev`
declarar falha antes mesmo de o servidor estar pronto.

O variant `yarn` do mesmo `Taskfile.yml`, gerado lado a lado pelo scaffold,
**não** tinha esse bug — já usava o padrão correto (`status: test -d
node_modules`). Ou seja, o bug era uma inconsistência de cópia entre os dois
variants no próprio template do CLI `wails3 init`, não algo introduzido por
este projeto.

**Correção (commit `d14218a`):** alinhar o variant `npm` ao mesmo padrão já
usado pelo `yarn`, trocando `generates` por `status`:

```yaml
# antes (build/Taskfile.yml, task install:frontend:deps:npm)
sources:
  - package.json
  - package-lock.json
generates:
  - node_modules
preconditions:
  - sh: npm version
    msg: "Looks like npm isn't installed. ..."

# depois
sources:
  - package.json
  - package-lock.json
status:
  - test -d node_modules
preconditions:
  - sh: npm version
    msg: "Looks like npm isn't installed. ..."
```

Com o fix, a task `install:frontend:deps:npm` passa a pular corretamente
`npm install` quando `node_modules` já existe, liberando o tempo necessário
para o Vite subir dentro da janela de retry de 5s — confirmado em 3 execuções
consecutivas de `wails3 dev` após o fix, todas conectando com folga (~1.5s de
margem).

**Relevância para a Etapa 2:** qualquer novo app gerado via `wails3 init`
(scaffold de produção, não descartável) provavelmente herda o mesmo bug no
`build/Taskfile.yml`, já que ele vem do próprio template do CLI, não deste
projeto. Aplicar a mesma correção (`generates` → `status: test -d
node_modules` na task `install:frontend:deps:npm`) assim que o scaffold da
Etapa 2 for gerado, antes de depender de `wails3 dev` para iteração.
