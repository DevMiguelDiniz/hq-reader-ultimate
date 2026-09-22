# Etapa 3b — Migração de Frontend + Religar ao Backend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrar `cmd/hqreader/frontend` de Vite+TS vanilla pra Vite+React+TypeScript+Tailwind CSS v4+shadcn/ui+Zustand, e religar a navegação de página ao contrato de estado server-side da Etapa 3a (`SetCurrentPage`/`CurrentPage`) — a Etapa 2 só rastreava o índice localmente no JS.

**Architecture:** Um `App.tsx` renderiza um componente `Reader` que lê/escreve estado através de uma store Zustand (`useReaderStore`). A store chama os bindings gerados do Wails (`Current`, `SetCurrentPage`) — nunca mantém o índice da página só no cliente. Componentes de UI vêm do shadcn/ui (Tailwind por baixo). Nenhuma lógica de zoom/pan/modos de página entra aqui — isso é Etapa 3c.

**Tech Stack:** Vite (já existente), React 19, TypeScript, Tailwind CSS v4 (`@tailwindcss/vite`), shadcn/ui, Zustand. Wails v3 `v3.0.0-beta.23` (pinado, inalterado desde a Etapa 0).

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — §2 (stack do frontend), §3.2 (estado no backend), §6 (Etapa 3).

## Global Constraints

- Stack do frontend fixada pelo spec original: React, TypeScript, Tailwind, shadcn/ui, Zustand. `Motion` fica **fora** deste plano — só entra na Etapa 3c quando existir uma transição animada de verdade (zoom/pan). Introduzi-la agora seria dependência sem uso.
- Zoom/pan, modo de página dupla, atalhos de teclado, placeholder de erro por página corrompida: **fora de escopo**, Etapa 3c.
- Nenhum framework de teste automatizado de frontend (vitest/jest) é introduzido — verificação de frontend neste projeto é manual/ponta-a-ponta via script de UI Automation, mesmo padrão já usado na Etapa 2. Decisão deliberada, não omissão.
- Toda navegação de página precisa chamar `SetCurrentPage` no backend — nunca só atualizar um índice local. Esse é o requisito central desta etapa (fecha o motivo de a Etapa 3a existir).
- Bindings gerados (`cmd/hqreader/frontend/bindings/**`) nunca são editados à mão — são regenerados via `wails3 dev` e commitados como estão (mesma política da Etapa 2).
- Wails v3 pinado em `v3.0.0-beta.23` — não instalar `@latest`.
- Ambiente já conhecido: `wails3 dev` pode ocasionalmente not conectar dentro do timeout de ~5s por latência de spawn de processo nesta máquina, mesmo com o bug do Taskfile já corrigido nas Etapas 0/2 — tentar de novo se acontecer, não é bug novo.
- `go build ./...` na raiz do repo só funciona depois de `frontend/dist` existir (gerado por `npm run build`) — gap conhecido desde a Etapa 2, não é regressão deste plano.

---

### Task 1: React + Tailwind CSS v4 no Vite, limpeza do template antigo

**Files:**
- Modify: `cmd/hqreader/frontend/package.json`
- Modify: `cmd/hqreader/frontend/vite.config.ts`
- Modify: `cmd/hqreader/frontend/tsconfig.json`
- Modify: `cmd/hqreader/frontend/index.html`
- Create: `cmd/hqreader/frontend/src/index.css`
- Create: `cmd/hqreader/frontend/src/App.tsx`
- Create: `cmd/hqreader/frontend/src/main.tsx`
- Delete: `cmd/hqreader/frontend/src/main.ts` (órfão desde a Etapa 2 — ainda importava um `GreetService` que não existe mais, sinalizado mas adiado na revisão final da Etapa 2)
- Delete: `cmd/hqreader/frontend/public/style.css`, `cmd/hqreader/frontend/public/wails.png`, `cmd/hqreader/frontend/public/typescript.svg`, `cmd/hqreader/frontend/public/bg-desktop.jpg`, `cmd/hqreader/frontend/public/bg-mobile.jpg`, `cmd/hqreader/frontend/public/Inter-Medium.ttf`, `cmd/hqreader/frontend/"Inter Font License.txt"` (assets órfãos do template `vanilla` original, nunca referenciados desde a Etapa 2)

**Interfaces:**
- Produces: um app React renderizando dentro de `#root`, com Tailwind funcionando (classes utilitárias aplicadas), consumido pela Task 2 (shadcn/ui) e Task 4 (componente Reader).

- [ ] **Step 1: Instalar as dependências**

```bash
cd cmd/hqreader/frontend
npm install react react-dom
npm install -D @vitejs/plugin-react tailwindcss @tailwindcss/vite @types/react @types/react-dom
```

- [ ] **Step 2: Apagar os arquivos órfãos do template vanilla**

```bash
rm src/main.ts
rm public/style.css public/wails.png public/typescript.svg public/bg-desktop.jpg public/bg-mobile.jpg public/Inter-Medium.ttf
rm "Inter Font License.txt"
```

- [ ] **Step 3: Atualizar `vite.config.ts`**

```ts
import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [wails("./bindings"), react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
```

Se `@tailwindcss/vite` não existir com esse nome na versão instalada (Tailwind CSS muda de major version ocasionalmente), rodar `npm view tailwindcss version` e `npm view @tailwindcss/vite` pra confirmar a versão resolvida, e consultar `node_modules/@tailwindcss/vite/README.md` (se existir) ou `node_modules/tailwindcss/README.md` pra achar o nome/uso atual do plugin Vite oficial — ajustar o import acima conforme o que for encontrado, mantendo o resto da estrutura.

- [ ] **Step 4: Atualizar `tsconfig.json`** — adicionar suporte a JSX e o alias `@/*`

```json
{
  "compilerOptions": {
    "allowJs": true,

    "noEmit": true,
    "skipLibCheck": true,

    "module": "ESNext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "verbatimModuleSyntax": true,

    "jsx": "react-jsx",
    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"]
    },

    "lib": [
      "DOM",
      "DOM.Iterable",
      "ESNext",
    ],

    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": false,
    "noImplicitAny": false,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
  },
  "include": ["src", "bindings"]
}
```

- [ ] **Step 5: Criar `src/index.css`**

```css
@import "tailwindcss";
```

- [ ] **Step 6: Criar `src/App.tsx`** (placeholder mínimo pra confirmar que Tailwind está funcionando — a Task 4 substitui o conteúdo de verdade)

```tsx
export default function App() {
  return (
    <div className="flex h-screen items-center justify-center bg-neutral-900 text-neutral-100">
      <p>hqreader</p>
    </div>
  );
}
```

- [ ] **Step 7: Criar `src/main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./index.css";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("elemento #root nao encontrado em index.html");
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [ ] **Step 8: Substituir `index.html` inteiro**

```html
<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <title>hqreader</title>
</head>
<body>
  <div id="root"></div>
  <script type="module" src="/src/main.tsx"></script>
</body>
</html>
```

- [ ] **Step 9: Build e verificação manual**

```bash
npm run build
```
Esperado: sucesso, sem erros de TypeScript ou do plugin do Tailwind.

```bash
cd ../..
wails3 dev
```
(Se falhar por timeout de conexão do dev server, tentar de novo — ver Global Constraints.) Esperado: uma janela abre mostrando "hqreader" centralizado em fundo escuro (`bg-neutral-900`) — confirma que o React está montando E que a classe Tailwind está sendo aplicada de verdade (não só que o texto aparece sem estilo nenhum). Parar o processo (Ctrl+C) depois de confirmar.

- [ ] **Step 10: Commit**

```bash
cd cmd/hqreader/frontend
git add -A package.json package-lock.json vite.config.ts tsconfig.json index.html src/index.css src/App.tsx src/main.tsx
git rm src/main.ts public/style.css public/wails.png public/typescript.svg public/bg-desktop.jpg public/bg-mobile.jpg public/Inter-Medium.ttf "Inter Font License.txt"
git commit -m "hqreader: migra frontend para React + Tailwind CSS v4, remove orfaos do template vanilla"
```

---

### Task 2: shadcn/ui — inicializar e adicionar o componente Button

**Files:**
- Create: `cmd/hqreader/frontend/components.json` (gerado pelo CLI do shadcn)
- Create: `cmd/hqreader/frontend/src/lib/utils.ts` (gerado pelo CLI do shadcn — helper `cn()`)
- Create: `cmd/hqreader/frontend/src/components/ui/button.tsx` (gerado pelo CLI do shadcn)
- Modify: `cmd/hqreader/frontend/src/App.tsx` (usar o `Button` como fumaça de verificação)
- Modify: `cmd/hqreader/frontend/package.json` (o CLI adiciona `clsx`, `tailwind-merge`, `class-variance-authority`, `lucide-react` automaticamente)

**Interfaces:**
- Consumes: Tailwind CSS v4 já configurado (Task 1).
- Produces: `import { Button } from "@/components/ui/button"` — consumido pela Task 4 (componente Reader, botões anterior/próxima).

- [ ] **Step 1: Inicializar o shadcn/ui**

```bash
cd cmd/hqreader/frontend
npx shadcn@latest init -d
```
A flag `-d` (defaults) evita perguntas interativas. Se essa flag não existir na versão resolvida pelo `npx`, rodar `npx shadcn@latest init --help` pra achar a flag não-interativa equivalente (procurar por algo como `--yes`, `--defaults`, ou `--force`) e usar essa — o objetivo é completar o init sem prompts interativos, já que este ambiente não tem um terminal interativo real disponível pra esse comando.

Confirmar que `components.json` foi criado na raiz de `cmd/hqreader/frontend/` e que `src/lib/utils.ts` existe com uma função `cn()` exportada.

- [ ] **Step 2: Adicionar o componente Button**

```bash
npx shadcn@latest add button
```
Confirmar que `src/components/ui/button.tsx` foi criado, exportando `Button` (e provavelmente `buttonVariants`).

- [ ] **Step 3: Usar o Button em `App.tsx` como verificação de fumaça**

Substituir o conteúdo de `src/App.tsx`:

```tsx
import { Button } from "@/components/ui/button";

export default function App() {
  return (
    <div className="flex h-screen items-center justify-center gap-4 bg-neutral-900 text-neutral-100">
      <p>hqreader</p>
      <Button>Testar</Button>
    </div>
  );
}
```

- [ ] **Step 4: Build e verificação manual**

```bash
npm run build
```
Esperado: sucesso.

```bash
cd ../..
wails3 dev
```
Esperado: a janela mostra "hqreader" e um botão "Testar" com o estilo visual do shadcn/ui (bordas arredondadas, cor de fundo do botão diferente do fundo da página — confirma que as classes do componente gerado estão sendo processadas pelo Tailwind corretamente, não só que o `<button>` aparece sem estilo). Parar o processo depois de confirmar.

- [ ] **Step 5: Commit**

```bash
cd cmd/hqreader/frontend
git add -A components.json src/lib/utils.ts src/components/ui/button.tsx src/App.tsx package.json package-lock.json
git commit -m "hqreader: inicializa shadcn/ui e adiciona o componente Button"
```

---

### Task 3: Store Zustand ligada a `SetCurrentPage`/`CurrentPage`

**Files:**
- Modify: `cmd/hqreader/frontend/package.json` (adicionar `zustand`)
- Create: `cmd/hqreader/frontend/src/store/readerStore.ts`
- Modify (regenerado pelo `wails3 dev`, não editado à mão): `cmd/hqreader/frontend/bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service.ts` e `models.ts` (ganham `SetCurrentPage`/`CurrentPage`, que ainda não existiam nos bindings commitados — a Etapa 3a nunca rodou `wails3 dev`, só mexeu no backend Go)

**Interfaces:**
- Consumes: os bindings gerados `Current()`, `SetCurrentPage(bookID, index)` (regenerados neste task a partir de `internal/reader.Service`, Etapa 3a).
- Produces: `useReaderStore` (hook Zustand) com o shape `{ bookID: string | null; pageCount: number; currentPage: number; loading: boolean; error: string | null; loadCurrentBook: () => Promise<void>; goToPage: (index: number) => Promise<void> }` — consumido pela Task 4 (componente Reader).

- [ ] **Step 1: Instalar o Zustand**

```bash
cd cmd/hqreader/frontend
npm install zustand
```

- [ ] **Step 2: Regenerar os bindings do Wails**

```bash
cd ../..
wails3 dev
```
(Retry se der timeout de conexão — ver Global Constraints.) Deixar rodar por alguns segundos pra gerar os bindings, depois parar (Ctrl+C).

Inspecionar o arquivo regenerado:
```bash
cat cmd/hqreader/frontend/bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service.ts
```
Confirmar que agora existem `SetCurrentPage(bookID: string, index: number): $CancellablePromise<void>` e `CurrentPage(bookID: string): $CancellablePromise<number>` (nomes e assinaturas devem corresponder exatamente aos métodos Go `Service.SetCurrentPage`/`Service.CurrentPage` da Etapa 3a — se os nomes gerados forem diferentes do assumido aqui, usar os nomes reais no Step 3 abaixo).

- [ ] **Step 3: Criar `src/store/readerStore.ts`**

```ts
import { create } from "zustand";
import {
  Current,
  SetCurrentPage,
} from "../../bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service";

interface ReaderState {
  bookID: string | null;
  pageCount: number;
  currentPage: number;
  loading: boolean;
  error: string | null;
  loadCurrentBook: () => Promise<void>;
  goToPage: (index: number) => Promise<void>;
}

export const useReaderStore = create<ReaderState>((set, get) => ({
  bookID: null,
  pageCount: 0,
  currentPage: 0,
  loading: true,
  error: null,

  // loadCurrentBook consulta o último livro aberto (Current()) e registra a
  // página inicial (0) no backend via SetCurrentPage — o estado da página
  // sempre vive no servidor, nunca só neste store (§3.2 do spec).
  loadCurrentBook: async () => {
    set({ loading: true, error: null });
    try {
      const result = await Current();
      set({
        bookID: result.BookID,
        pageCount: result.PageCount,
        currentPage: 0,
        loading: false,
      });
      await SetCurrentPage(result.BookID, 0);
    } catch (err) {
      set({ loading: false, error: String(err) });
    }
  },

  // goToPage atualiza o estado local (pra UI responder na hora) e chama
  // SetCurrentPage em seguida — a fonte da verdade é o backend.
  goToPage: async (index: number) => {
    const { bookID, pageCount } = get();
    if (!bookID || index < 0 || index >= pageCount) {
      return;
    }
    set({ currentPage: index });
    try {
      await SetCurrentPage(bookID, index);
    } catch (err) {
      set({ error: String(err) });
    }
  },
}));
```

- [ ] **Step 4: Build**

```bash
cd cmd/hqreader/frontend
npm run build
```
Esperado: sucesso — nenhum consumidor de `useReaderStore` existe ainda (isso é a Task 4), então este build só confirma que o arquivo compila e os tipos batem com os bindings regenerados.

- [ ] **Step 5: Commit**

```bash
git add -A package.json package-lock.json src/store/readerStore.ts bindings/
git commit -m "hqreader: store Zustand ligada a SetCurrentPage/CurrentPage do backend"
```

---

### Task 4: Componente `Reader` — substitui o script vanilla da Etapa 2

**Files:**
- Create: `cmd/hqreader/frontend/src/components/Reader.tsx`
- Modify: `cmd/hqreader/frontend/src/App.tsx`

**Interfaces:**
- Consumes: `useReaderStore` (Task 3), `Button` (Task 2).
- Produces: a UI de leitura de verdade (ainda sem zoom/pan/atalhos — isso é Etapa 3c). Nenhuma task futura *deste* plano depende de algo novo aqui; a Task 5 só verifica o comportamento.

- [ ] **Step 1: Criar `src/components/Reader.tsx`**

```tsx
import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { useReaderStore } from "@/store/readerStore";

export default function Reader() {
  const {
    bookID,
    pageCount,
    currentPage,
    loading,
    error,
    loadCurrentBook,
    goToPage,
  } = useReaderStore();

  // loadCurrentBook é chamado uma vez ao montar. Sob React StrictMode (dev),
  // o efeito roda duas vezes — isso é seguro aqui porque Current() e
  // SetCurrentPage(id, 0) são ambos idempotentes (Etapa 3a garante isso).
  useEffect(() => {
    loadCurrentBook();
  }, [loadCurrentBook]);

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-neutral-900 text-neutral-100">
        Carregando...
      </div>
    );
  }

  if (error || !bookID) {
    return (
      <div className="flex h-screen items-center justify-center bg-neutral-900 text-red-400">
        Erro ao abrir o livro: {error}
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col items-center justify-center gap-4 bg-neutral-900 text-neutral-100">
      <img
        src={`/page/${bookID}/${currentPage}`}
        alt="pagina"
        className="max-h-[80vh] max-w-[90vw]"
      />
      <div className="flex items-center gap-4">
        <Button
          onClick={() => goToPage(currentPage - 1)}
          disabled={currentPage <= 0}
        >
          Anterior
        </Button>
        <span>
          {currentPage + 1} / {pageCount}
        </span>
        <Button
          onClick={() => goToPage(currentPage + 1)}
          disabled={currentPage >= pageCount - 1}
        >
          Proxima
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Atualizar `src/App.tsx`**

```tsx
import Reader from "@/components/Reader";

export default function App() {
  return <Reader />;
}
```

- [ ] **Step 3: Build**

```bash
cd cmd/hqreader/frontend
npm run build
```
Esperado: sucesso.

- [ ] **Step 4: Verificação manual**

```bash
cd ../..
```
No PowerShell, com uma variável `HQREADER_BOOK` apontando pra um `.cbz` real de teste (ver Task 5 pra como gerar um):
```powershell
$env:HQREADER_BOOK = "C:\caminho\completo\para\teste.cbz"
wails3 dev
```
Esperado: a primeira página carrega, o contador mostra "1 / N", os botões Anterior/Próxima navegam corretamente e ficam desabilitados nos limites (não é mais possível clicar além do início/fim). Parar o processo depois de confirmar — a verificação rigorosa com evidência real fica pra Task 5.

- [ ] **Step 5: Commit**

```bash
cd cmd/hqreader/frontend
git add -A src/components/Reader.tsx src/App.tsx
git commit -m "hqreader: componente Reader usa a store e o backend de verdade"
```

---

### Task 5: Verificação de ponta a ponta (dev e produção)

**Files:**
- Nenhum arquivo de produto novo — só um script de verificação e seu registro, seguindo exatamente o padrão já estabelecido na Etapa 2 (script real e completo + saída bruta commitados, não uma transcrição parcial).

**Interfaces:**
- Consumes: o app `cmd/hqreader` completo (Tasks 1-4 deste plano + Etapa 2/3a).

- [ ] **Step 1: Criar um CBZ de teste com várias páginas** (mesmo helper já usado na Etapa 2)

```bash
mkdir -p /tmp/hqreader-manual-test
cat <<'EOF' > /tmp/hqreader-manual-test/make_test_cbz.go
package main

import (
	"archive/zip"
	"os"
)

func main() {
	f, _ := os.Create(os.Args[1])
	defer f.Close()
	zw := zip.NewWriter(f)
	pages := [][]byte{
		{0xFF, 0xD8, 0xFF, 0xD9},
		{0xFF, 0xD8, 0xFF, 0xD9},
		{0xFF, 0xD8, 0xFF, 0xD9},
	}
	names := []string{"page1.jpg", "page2.jpg", "page3.jpg"}
	for i, data := range pages {
		w, _ := zw.Create(names[i])
		w.Write(data)
	}
	zw.Close()
}
EOF
go run /tmp/hqreader-manual-test/make_test_cbz.go /tmp/hqreader-manual-test/teste.cbz
```

- [ ] **Step 2: Escrever um script real de verificação de navegação (React)**

Criar `cmd/hqreader/verify-navigation-react.ps1`. Este script precisa ser completo e re-executável — não uma versão resumida. Adaptar (não reinventar do zero) o script `cmd/hqreader/verify-navigation.ps1` já commitado da Etapa 2 como ponto de partida: ele já resolve o processo/janela via UI Automation e sabe ler o texto do elemento `#counter`/contador e invocar os botões por nome. A estrutura visual dos botões mudou (agora são `<Button>` do shadcn/ui, que renderizam como `<button>` normal com o mesmo texto "Anterior"/"Proxima" — a busca por nome do elemento deve continuar funcionando sem mudança), então adapte o script existente em vez de escrever um novo do zero, ajustando só o que realmente precisar mudar (ex.: se a estrutura do DOM dentro do `<button>` mudou o suficiente pra quebrar a leitura de texto via UI Automation, investigar a árvore de acessibilidade real da janela rodando e ajustar).

O script deve cobrir, com evidência real capturada (não só narrada):
- Carregamento da primeira página (contador "1 / 3")
- "Proxima" duas vezes → contador "3 / 3", botão "Proxima" fica desabilitado (não avança mais)
- "Anterior" duas vezes → contador "1 / 3", botão "Anterior" fica desabilitado
- Confirmação de que a página `<img>` renderiza (elemento presente na árvore de acessibilidade)

- [ ] **Step 3: Rodar em modo dev e capturar a saída bruta**

```powershell
cd cmd/hqreader
$env:HQREADER_BOOK = "C:\caminho\completo\para\teste.cbz"
wails3 dev
```
(Retry se der timeout de conexão — ver Global Constraints.) Em outro terminal, rodar o script de verificação contra o processo aberto, redirecionando a saída real (não editada) pra um arquivo:
```powershell
.\verify-navigation-react.ps1 *> verify-navigation-react-dev-output.txt
```
Parar o `wails3 dev` depois.

- [ ] **Step 4: Repetir no build de produção**

```bash
wails3 build
```
Rodar o binário gerado (`bin/hqreader.exe`) com a mesma variável de ambiente `HQREADER_BOOK`, e rodar o mesmo script de verificação contra esse processo, capturando a saída bruta:
```powershell
.\verify-navigation-react.ps1 *> verify-navigation-react-prod-output.txt
```
Isso exercita o caminho de assets embutidos (`AssetFileServerFS` servindo `frontend/dist`), diferente do proxy do Vite usado em `wails3 dev` — mesmo motivo pelo qual a Etapa 2 verificou os dois caminhos separadamente.

- [ ] **Step 5: Atualizar `MANUAL_VERIFICATION.md`**

Adicionar uma seção ao final de `cmd/hqreader/MANUAL_VERIFICATION.md` (já existe da Etapa 2 — não substituir o conteúdo anterior, só acrescentar):

```markdown
## Etapa 3b — React + Tailwind + shadcn/ui + Zustand

**Data:** <preencher>
**Cenário:** mesmo CBZ sintético de 3 páginas, agora com a UI em React chamando
SetCurrentPage no backend a cada navegação (não mais um índice só local).

Script real: `verify-navigation-react.ps1`. Saída bruta:
`verify-navigation-react-dev-output.txt` (wails3 dev),
`verify-navigation-react-prod-output.txt` (wails3 build).

- [ ] Primeira página carrega, contador "1 / 3"
- [ ] Navegação Anterior/Proxima funciona e desabilita nos limites
- [ ] Mesmo comportamento em produção (build embutido)
- [ ] Nenhum erro observado no console/saída relacionado a SetCurrentPage

**Observações:** <qualquer desvio do esperado, ou "nenhum">
```

Preencher com o resultado real observado nos Steps 3-4.

- [ ] **Step 6: Commit**

```bash
cd cmd/hqreader
git add verify-navigation-react.ps1 verify-navigation-react-dev-output.txt verify-navigation-react-prod-output.txt MANUAL_VERIFICATION.md
git commit -m "hqreader: verificacao de ponta a ponta do reader em React (dev e producao)"
```

---

## Self-Review

**Cobertura do spec:** migração da stack de frontend pra React/TS/Tailwind/shadcn/ui/Zustand (§2) → Tasks 1-2; estado da página vivendo no backend, nunca só no cliente (§3.2) → Task 3 (a store SEMPRE chama `SetCurrentPage`, nunca guarda o índice como única fonte de verdade) e Task 4 (o componente consome a store, não um estado React local paralelo); "leitor de verdade" navegando de página de verdade (§6, parte do escopo da Etapa 3 que cabe nesta metade) → Task 4; verificação rigorosa em dev e produção com evidência real, mesmo padrão da Etapa 2 → Task 5. Zoom/pan, modo de página dupla, atalhos, placeholder de erro, e a biblioteca Motion ficam explicitamente fora (Global Constraints) — não há requisito do §6 pra essa parte sem task correspondente, porque esses itens pertencem à Etapa 3c por decisão já tomada.

**Placeholders:** nenhum "TBD"/"TODO" solto. As duas partes com investigação-antes-de-codar (nome real do pacote do plugin Vite do Tailwind na Task 1, flag não-interativa do CLI do shadcn na Task 2, nomes reais de `SetCurrentPage`/`CurrentPage` nos bindings regenerados na Task 3) seguem o mesmo padrão já validado nas Etapas 0-3a: código completo como ponto de partida, com o passo de verificação explícito antes de qualquer coisa que dependa do resultado.

**Consistência de tipos:** `useReaderStore` com o shape `{bookID, pageCount, currentPage, loading, error, loadCurrentBook, goToPage}` definido na Task 3 é consumido com os mesmos nomes exatos na Task 4. `Button` de `@/components/ui/button` (Task 2) é importado com o mesmo caminho em `App.tsx` (Task 2) e `Reader.tsx` (Task 4). O alias `@/*` configurado no `tsconfig.json`/`vite.config.ts` da Task 1 é usado de forma consistente em todos os imports das Tasks 2 e 4 (nunca um caminho relativo `../../` pra código dentro de `src/`, só pros bindings gerados, que ficam fora de `src/`).

