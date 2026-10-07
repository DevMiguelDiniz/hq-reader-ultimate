# Etapa 3c — Interações do Leitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adicionar ao leitor zoom/pan, modo de página única/dupla, atalhos de teclado, placeholder de erro por página e transição de troca de página com Motion — só frontend, nenhuma mudança no backend Go.

**Architecture:** Duas funções puras (`lib/computeSpread.ts` decide quais páginas compõem a visão atual; `lib/viewTransform.ts` faz a matemática de zoom/pan) alimentam a store Zustand existente (`readerStore.ts`), que ganha `zoom`, `pan`, `metrics`, `pageMode`, `pageAspects` e as ações de navegação por visão. Um componente `PageView` renderiza a visão (uma página ou um par) com `transform` CSS, trata `onLoad`/`onError` por página e captura roda/arrasto; um hook `useReaderShortcuts` cuida do teclado; `Reader.tsx` compõe tudo.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, shadcn/ui (`Button`), Zustand, `lucide-react` (já instalado), `motion` (nova dependência, só na Task 5). Wails v3 `v3.0.0-beta.23` (pinado, inalterado).

**Spec:** [docs/superpowers/specs/2026-09-16-hq-reader-design.md](../specs/2026-09-16-hq-reader-design.md) — **§10 (Etapa 3c)** é a fonte de verdade desta etapa; §3.6 (tratamento de erro por página) e §4 (spreads) dão o contexto.

## Precondition

Este plano foi escrito contra o código da `main` **depois** do merge da branch `claude/sweet-bardeen-eb792f` (commit `8bfbec1`, "falha de navegacao nao derruba mais o reader inteiro"), que adiciona `navError`/`clearNavError` à store e um banner de erro em `Reader.tsx`. Os trechos de `Reader.tsx` abaixo já incluem esse banner. **A Task 3 começa verificando que esse commit está na branch de trabalho; se não estiver, PARE e reporte BLOCKED** — não reimplemente o `navError` neste plano.

## Global Constraints

- Wails v3 pinado em `v3.0.0-beta.23`; nunca instalar `@latest` de nenhum pacote Wails (spec §2).
- Esta etapa **não muda o backend Go** (spec §10 intro). Nenhum arquivo em `internal/` ou `cmd/hqreader/main.go` é tocado.
- Nenhum framework de teste de frontend (vitest/jest): política deliberada do projeto (spec §7, §10.5). Verificação é por script PowerShell de UI Automation, só em Windows. Checagens descartáveis (scratch, **nunca commitadas**) são permitidas.
- Zoom: mínimo 50%, máximo 400%, 100% = ajustar à tela; roda do mouse dá zoom centrado no cursor (~15% por passo); arrastar só move a imagem quando ela excede o viewport; trocar de página reseta zoom e pan para 100% (spec §10.1).
- Modo duplo: página 1 (índice 0) sempre sozinha; página com `naturalWidth > naturalHeight` é spread e sempre aparece sozinha (spec §10.1 e §4). Modo de página é só em memória, padrão `single` (spec §10.1).
- Atalhos: `←`/`A` anterior, `→`/`D` próxima, `Espaço` próxima, `+`/`=` zoom in, `-` zoom out, `0` reseta zoom e pan, `M` alterna página única/dupla (spec §10.2).
- Placeholder de erro: ícone de aviso, texto "Não foi possível carregar esta página", botão "Tentar novamente"; Anterior/Próxima continuam habilitados; no modo duplo só a metade que falhou vira placeholder (spec §10.1).
- Motion só na troca de página: fade de ~150ms via `AnimatePresence`, `prefers-reduced-motion` torna a troca instantânea; zoom/pan **não** passa pelo Motion (spec §10.1).
- Validação em WebKitGTK/Linux está fora de escopo (spec §10.6).
- Ambiente: `wails3 dev` às vezes não conecta dentro do timeout de ~5s por latência de spawn nesta máquina — tente de novo, não é bug novo. `go build ./...` na raiz só funciona depois de `cmd/hqreader/frontend/dist` existir (`npm run build`). Um worktree novo não tem `node_modules`: rode `npm install` em `cmd/hqreader/frontend` antes de qualquer build.
- `npm run build` (Vite/esbuild) **não faz checagem de tipos** e o `typescript@^4.9.3` do projeto não consegue ler o `tsconfig.json` (pendência conhecida, fora desta etapa). Melhor esforço de tipos: de `cmd/hqreader/frontend`, `npx -p typescript@5 tsc --noEmit -p tsconfig.json`. Erros em arquivos que este plano não toca são pré-existentes; erros novos nos arquivos tocados devem ser corrigidos. Se o comando não conseguir rodar, diga isso no relatório e confie em leitura cuidadosa.
- Strings de controles da UI são ASCII sem acento (`Anterior`, `Proxima`, `Pagina unica`, `Pagina dupla`, `Tentar novamente`), como `Anterior`/`Proxima` já são; os scripts de UI Automation localizam controles pelo nome. A mensagem de erro do placeholder mantém os acentos da spec (o script a localiza por trecho ASCII).

## Decisões que a spec deixa em aberto (tomadas neste plano — revise)

1. **Pareamento após um spread:** slots de paridade fixa — os pares são (1,2), (3,4), (5,6)... (índices 0-based), e se **qualquer** página do slot for spread, as **duas** aparecem sozinhas. Isso é local (não depende do histórico de navegação) mas pode deixar uma página vizinha de um spread sozinha. Alternativa descartada: pareamento guloso, que depende de conhecer a proporção de todas as páginas anteriores.
2. **Página ainda sem proporção conhecida no modo duplo:** a visão fica invisível (mas as imagens carregam) até as páginas do slot reportarem `onLoad`/`onError`; página que falhou conta como "conhecida, não-spread".
3. **Contador no modo duplo mostra a faixa:** `2-3 / 10`; página sozinha mostra `4 / 10`. Só um nó de texto por dado (contador, zoom `100%`).
4. **Botão visível de modo** (`Pagina unica` / `Pagina dupla`, clica para alternar) além da tecla `M`; e um texto de zoom (`100%`) ao lado do contador. A spec só define a tecla e o zoom por roda; esses dois elementos dão feedback visível e dão ao script de UI Automation algo observável.
5. **Alternar o modo reseta zoom e pan** (o layout muda de tamanho).
6. **Passo da roda:** `fator = exp(-deltaY * 0.0015)` (~15% por passo padrão de 100); teclas `+`/`-` usam fator fixo 1.15 centrado no viewport.
7. **Teclas com Ctrl/Alt/Meta são ignoradas** (não sequestram atalhos do sistema); `Espaço` tira o foco do botão focado antes de agir, para o clique de teclado do botão não avançar uma segunda vez.
8. **Próxima/anterior são ignoradas enquanto a visão atual está `pending`** (a proporção de alguma página do slot ainda não foi medida). Sem isso, um `→` rápido no modo duplo avançaria sobre um spread ainda não medido e pularia uma página. Custo: durante os poucos ms de carregamento de um slot novo, uma tecla é descartada, não enfileirada. O script de verificação aguarda as imagens aparecerem antes de cada tecla no modo duplo por causa disso.
9. O índice guardado no backend (`SetCurrentPage`) no modo duplo é o **primeiro** índice da visão; a janela de prefetch N±2 do backend não é ajustada para pares (não há mudança de backend nesta etapa).

## Review Focus

Entradas/condições que a spec implica mas nenhuma task testaria por padrão, mais prováveis primeiro — cada uma tem seu teste na task dona do código:

1. **Livro de 1 página, de 2 páginas, `currentPage` fora do intervalo** no modo duplo (visão vazia/sozinha, "próxima" e "anterior" devolvem `null`, sem exceção) → scratch check da **Task 2**.
2. **Proporção exatamente 1 (quadrada) não é spread; página com erro conta como não-spread** → scratch check da **Task 2** (e pareamento com metade quebrada na **Task 7**, passo 6.7).
3. **Janela redimensionada/maximizada com zoom ativo** (pan antigo ficaria fora do limite e a imagem sumiria) → re-clamp do pan via `ResizeObserver` → **Task 4** (código), verificado na **Task 7**, passo 6.5.
4. **Tecla segurada/rajada** (`{RIGHT 5}`) no modo único, `→` rápido no modo duplo sobre uma página ainda não medida (não pode pular um spread — Decisão 8, guarda `view.pending` na **Task 3**), e `Espaço` com um botão focado depois de um clique real (não pode avançar duas páginas) → **Task 6** (código), verificado na **Task 7**, passos 6.2 e 6.6.
5. **Combinações com modificador** (`Ctrl+→`) não podem navegar → **Task 6** (código), verificado na **Task 7**, passo 6.2.

---

### Task 1: Gerador do CBZ de verificação

**Files:**
- Create: `cmd/hqreader/make-verify-cbz.go` (`//go:build ignore` — fora do build normal; roda só com `go run`)

**Interfaces:**
- Produces: um CBZ de **10 páginas**, usado pelas Tasks 4–7 (checagens manuais e script final). Índices 0-based: `0` capa retrato, `1,2,3` retratos, `4` **spread** (paisagem), `5` retrato, `6` **corrompida** (bytes que não são PNG, nome `.png`), `7,8,9` retratos. Nomes `page01.png` … `page10.png`. Pareamento esperado no modo duplo (Decisão 1): visões `[0] [1,2] [3] [4] [5,6] [7,8] [9]`.

- [ ] **Step 1: Criar `cmd/hqreader/make-verify-cbz.go`**

```go
//go:build ignore

// Gera o CBZ sintetico de 10 paginas usado por verify-reader-interactions.ps1
// e pelas verificacoes manuais da Etapa 3c. Nao faz parte do build.
//
// Uso (de cmd/hqreader): go run make-verify-cbz.go <saida.cbz>
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

type page struct {
	name    string
	w, h    int
	corrupt bool
}

func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "uso: go run make-verify-cbz.go <saida.cbz>")
		os.Exit(1)
	}

	pages := []page{
		{name: "page01.png", w: 600, h: 900},                // 0 capa
		{name: "page02.png", w: 600, h: 900},                // 1
		{name: "page03.png", w: 600, h: 900},                // 2
		{name: "page04.png", w: 600, h: 900},                // 3
		{name: "page05.png", w: 1200, h: 900},               // 4 spread
		{name: "page06.png", w: 600, h: 900},                // 5
		{name: "page07.png", w: 600, h: 900, corrupt: true}, // 6 corrompida
		{name: "page08.png", w: 600, h: 900},                // 7
		{name: "page09.png", w: 600, h: 900},                // 8
		{name: "page10.png", w: 600, h: 900},                // 9
	}

	f, err := os.Create(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for i, p := range pages {
		w, err := zw.Create(p.name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var data []byte
		if p.corrupt {
			data = []byte("isto nao e um png")
		} else {
			data = solidPNG(p.w, p.h, color.RGBA{R: uint8(40 + i*20), G: uint8(200 - i*15), B: 120, A: 255})
		}
		if _, err := w.Write(data); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Gerar o CBZ e conferir com o `hqcli`**

Run (de `cmd/hqreader`, PowerShell): `go run make-verify-cbz.go $env:TEMP\hqreader-interactions.cbz`
Run (da raiz do repo): `go run ./cmd/hqcli list $env:TEMP\hqreader-interactions.cbz`
Expected: 10 linhas, índices `0`–`9`, nomes `page01.png` … `page10.png`, `ContentType` `image/png` em todas (a página 6 lista como PNG apesar de corrompida — o archive só filtra por extensão).

- [ ] **Step 3: Garantir que o arquivo não entra no build normal**

Run (da raiz): `go vet ./cmd/hqreader` (precisa de `cmd/hqreader/frontend/dist`; se não existir, rode antes `npm install && npm run build` em `cmd/hqreader/frontend`).
Expected: sem erros, sem mencionar `make-verify-cbz.go` (o `//go:build ignore` o exclui; sem a tag haveria `main redeclared`).

- [ ] **Step 4: Commit**

```bash
git add cmd/hqreader/make-verify-cbz.go
git commit -m "hqreader: gerador do CBZ sintetico de verificacao da Etapa 3c"
```

---

### Task 2: Lógica pura — visão da página e matemática de zoom/pan

**Files:**
- Create: `cmd/hqreader/frontend/src/lib/computeSpread.ts`
- Create: `cmd/hqreader/frontend/src/lib/viewTransform.ts`
- Scratch (não commitar): `cmd/hqreader/frontend/scratch-check.ts`

**Interfaces:**
- Produces (consumido pelas Tasks 3, 4, 5, 6):
  - `computeSpread.ts`: `type PageAspect = number | "error"`, `type PageMode = "single" | "double"`, `type PageAspects = Record<number, PageAspect | undefined>`, `interface ReaderView { indices: number[]; pending: boolean }`, `isSpread(aspect)`, `computeSpread(currentPage, pageMode, pageAspects, pageCount): ReaderView`, `nextPageIndex(view, pageCount): number | null`, `prevPageIndex(view, pageMode, pageAspects, pageCount): number | null`, `viewLabel(view): string`.
  - `viewTransform.ts`: `MIN_ZOOM`, `MAX_ZOOM`, `ZOOM_KEY_STEP`, `interface Point`, `interface Size`, `clampZoom`, `clampPan(pan, zoom, content, viewport): Point`, `zoomAt(zoom, pan, factor, cursor, content, viewport): { zoom: number; pan: Point }`.

- [ ] **Step 1: Criar `src/lib/computeSpread.ts`**

```ts
export type PageAspect = number | "error";
export type PageMode = "single" | "double";
export type PageAspects = Record<number, PageAspect | undefined>;

export interface ReaderView {
  indices: number[];
  pending: boolean;
}

// aspect é naturalWidth/naturalHeight; paisagem (largura > altura) é spread.
export function isSpread(aspect: PageAspect | undefined): boolean {
  return typeof aspect === "number" && aspect > 1;
}

// computeSpread devolve as páginas da visão que contém currentPage. No modo
// duplo os slots têm paridade fixa — (1,2), (3,4), (5,6)... em índices
// 0-based, com o índice 0 (capa) sempre sozinho — e se qualquer página do
// slot for spread as duas aparecem sozinhas. pending=true enquanto o slot
// ainda tem página sem proporção conhecida (a decisão de parear só é
// definitiva depois do onLoad/onError das páginas envolvidas).
export function computeSpread(
  currentPage: number,
  pageMode: PageMode,
  pageAspects: PageAspects,
  pageCount: number,
): ReaderView {
  if (pageCount <= 0 || currentPage < 0 || currentPage >= pageCount) {
    return { indices: [], pending: false };
  }
  if (pageMode === "single" || currentPage === 0) {
    return { indices: [currentPage], pending: false };
  }

  const leader = currentPage % 2 === 1 ? currentPage : currentPage - 1;
  const slot = leader + 1 < pageCount ? [leader, leader + 1] : [leader];
  if (slot.length === 1 || slot.some((i) => isSpread(pageAspects[i]))) {
    return { indices: [currentPage], pending: false };
  }
  return {
    indices: slot,
    pending: slot.some((i) => pageAspects[i] === undefined),
  };
}

export function nextPageIndex(
  view: ReaderView,
  pageCount: number,
): number | null {
  if (view.indices.length === 0) {
    return null;
  }
  const next = view.indices[view.indices.length - 1] + 1;
  return next < pageCount ? next : null;
}

export function prevPageIndex(
  view: ReaderView,
  pageMode: PageMode,
  pageAspects: PageAspects,
  pageCount: number,
): number | null {
  if (view.indices.length === 0) {
    return null;
  }
  const before = view.indices[0] - 1;
  if (before < 0) {
    return null;
  }
  return computeSpread(before, pageMode, pageAspects, pageCount).indices[0];
}

// viewLabel é o texto de página do contador, 1-based: "4" ou "2-3".
export function viewLabel(view: ReaderView): string {
  if (view.indices.length === 0) {
    return "0";
  }
  const first = view.indices[0] + 1;
  const last = view.indices[view.indices.length - 1] + 1;
  return first === last ? `${first}` : `${first}-${last}`;
}
```

- [ ] **Step 2: Criar `src/lib/viewTransform.ts`**

```ts
export const MIN_ZOOM = 0.5;
export const MAX_ZOOM = 4;
export const ZOOM_KEY_STEP = 1.15;

export interface Point {
  x: number;
  y: number;
}

export interface Size {
  width: number;
  height: number;
}

export function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

// clampPan impede a borda da imagem de entrar no viewport: o deslocamento
// máximo em cada eixo é metade do que o conteúdo ampliado excede o viewport
// (zero se ele cabe, e aí a imagem fica centralizada).
export function clampPan(
  pan: Point,
  zoom: number,
  content: Size,
  viewport: Size,
): Point {
  const maxX = Math.max(0, (content.width * zoom - viewport.width) / 2);
  const maxY = Math.max(0, (content.height * zoom - viewport.height) / 2);
  // "+ 0" normaliza -0 para 0.
  return {
    x: Math.min(maxX, Math.max(-maxX, pan.x)) + 0,
    y: Math.min(maxY, Math.max(-maxY, pan.y)) + 0,
  };
}

// zoomAt aplica factor ao zoom mantendo fixo o ponto sob o cursor. cursor é
// relativo ao centro do viewport (o transform-origin do conteúdo é o centro).
export function zoomAt(
  zoom: number,
  pan: Point,
  factor: number,
  cursor: Point,
  content: Size,
  viewport: Size,
): { zoom: number; pan: Point } {
  const nextZoom = clampZoom(zoom * factor);
  const ratio = nextZoom / zoom;
  const nextPan = {
    x: cursor.x - (cursor.x - pan.x) * ratio,
    y: cursor.y - (cursor.y - pan.y) * ratio,
  };
  return {
    zoom: nextZoom,
    pan: clampPan(nextPan, nextZoom, content, viewport),
  };
}
```

- [ ] **Step 3: Escrever o scratch check (NÃO commitar)**

Criar `cmd/hqreader/frontend/scratch-check.ts`:

```ts
import assert from "node:assert/strict";
import {
  computeSpread,
  isSpread,
  nextPageIndex,
  prevPageIndex,
  viewLabel,
} from "./src/lib/computeSpread.ts";
import { clampPan, clampZoom, zoomAt } from "./src/lib/viewTransform.ts";

// 10 páginas: índice 4 é spread, índice 6 falhou ao carregar, o resto é retrato.
const aspects = {
  0: 0.66, 1: 0.66, 2: 0.66, 3: 0.66, 4: 1.33,
  5: 0.66, 6: "error", 7: 0.66, 8: 0.66, 9: 0.66,
};

const idx = (p: number) => computeSpread(p, "double", aspects, 10).indices;
assert.deepEqual(idx(0), [0]);
assert.deepEqual(idx(1), [1, 2]);
assert.deepEqual(idx(2), [1, 2]);
assert.deepEqual(idx(3), [3]);
assert.deepEqual(idx(4), [4]);
assert.deepEqual(idx(5), [5, 6]);
assert.deepEqual(idx(6), [5, 6]);
assert.deepEqual(idx(7), [7, 8]);
assert.deepEqual(idx(8), [7, 8]);
assert.deepEqual(idx(9), [9]);
assert.deepEqual(computeSpread(3, "single", aspects, 10).indices, [3]);

// Caminhando para a frente a partir da capa, as visões começam em:
const forward: number[] = [];
let p: number | null = 0;
while (p !== null) {
  forward.push(p);
  p = nextPageIndex(computeSpread(p, "double", aspects, 10), 10);
}
assert.deepEqual(forward, [0, 1, 3, 4, 5, 7, 9]);

// ...e para trás a partir da última:
const backward: number[] = [];
p = 9;
while (p !== null) {
  backward.push(p);
  p = prevPageIndex(computeSpread(p, "double", aspects, 10), "double", aspects, 10);
}
assert.deepEqual(backward, [9, 7, 5, 4, 3, 1, 0]);

// pending: slot com página de proporção desconhecida.
assert.deepEqual(computeSpread(1, "double", {}, 10), { indices: [1, 2], pending: true });
assert.deepEqual(computeSpread(1, "double", { 1: 0.66 }, 10), { indices: [1, 2], pending: true });
assert.deepEqual(computeSpread(1, "double", { 1: 0.66, 2: 0.66 }, 10), { indices: [1, 2], pending: false });
assert.deepEqual(computeSpread(0, "double", {}, 10), { indices: [0], pending: false });
// Spread conhecido decide sem esperar a outra página.
assert.deepEqual(computeSpread(1, "double", { 1: 1.5 }, 10), { indices: [1], pending: false });
assert.deepEqual(computeSpread(2, "double", { 1: 1.5 }, 10), { indices: [2], pending: false });

// Proporção exatamente 1 não é spread; erro não é spread.
assert.equal(isSpread(1), false);
assert.equal(isSpread("error"), false);
assert.equal(isSpread(undefined), false);
assert.equal(isSpread(1.01), true);
assert.deepEqual(computeSpread(1, "double", { 1: 1, 2: 1 }, 10).indices, [1, 2]);

// Entradas de borda: 1 página, 2 páginas, fora do intervalo, 0 páginas.
assert.deepEqual(computeSpread(0, "double", {}, 1), { indices: [0], pending: false });
assert.equal(nextPageIndex(computeSpread(0, "double", {}, 1), 1), null);
assert.equal(prevPageIndex(computeSpread(0, "double", {}, 1), "double", {}, 1), null);
assert.deepEqual(computeSpread(1, "double", { 1: 0.66 }, 2), { indices: [1], pending: false });
assert.deepEqual(computeSpread(5, "double", {}, 3), { indices: [], pending: false });
assert.deepEqual(computeSpread(-1, "double", {}, 3), { indices: [], pending: false });
assert.deepEqual(computeSpread(0, "double", {}, 0), { indices: [], pending: false });
assert.equal(nextPageIndex({ indices: [], pending: false }, 3), null);
assert.equal(prevPageIndex({ indices: [], pending: false }, "double", {}, 3), null);

// viewLabel
assert.equal(viewLabel({ indices: [0], pending: false }), "1");
assert.equal(viewLabel({ indices: [1, 2], pending: false }), "2-3");
assert.equal(viewLabel({ indices: [], pending: false }), "0");

// viewTransform
assert.equal(clampZoom(10), 4);
assert.equal(clampZoom(0.1), 0.5);
assert.equal(clampZoom(1.7), 1.7);

const content = { width: 600, height: 1000 };
const viewport = { width: 900, height: 600 };
assert.deepEqual(clampPan({ x: 50, y: 50 }, 1, { width: 300, height: 500 }, viewport), { x: 0, y: 0 });
assert.deepEqual(clampPan({ x: -50, y: 50 }, 1, { width: 300, height: 500 }, viewport), { x: 0, y: 0 });
assert.deepEqual(clampPan({ x: 50, y: 500 }, 2, content, viewport), { x: 50, y: 700 });
assert.deepEqual(clampPan({ x: 500, y: -900 }, 2, content, viewport), { x: 150, y: -700 });

const z = zoomAt(1, { x: 0, y: 0 }, 2, { x: 100, y: 50 }, content, viewport);
assert.deepEqual(z, { zoom: 2, pan: { x: -100, y: -50 } });
// O ponto sob o cursor fica fixo: q = (cursor - pan0)/zoom0; cursor = pan1 + zoom1*q.
assert.equal(z.pan.x + z.zoom * 100, 100);
assert.equal(z.pan.y + z.zoom * 50, 50);

assert.equal(zoomAt(3, { x: 0, y: 0 }, 2, { x: 0, y: 0 }, content, viewport).zoom, 4);
assert.deepEqual(zoomAt(1, { x: 40, y: 40 }, 0.1, { x: 0, y: 0 }, { width: 300, height: 500 }, viewport), { zoom: 0.5, pan: { x: 0, y: 0 } });

console.log("scratch-check ok");
```

- [ ] **Step 4: Rodar o scratch check**

Run (de `cmd/hqreader/frontend`): `node scratch-check.ts`
Expected: imprime `scratch-check ok` (o Node 24 remove os tipos sozinho; um `ExperimentalWarning` sobre type stripping é aceitável). Qualquer `AssertionError` é um bug na lógica das duas bibliotecas — corrija a biblioteca, não o teste, a menos que o teste contradiga a spec §10.
Se o Node recusar o `.ts`, use o fallback: `npx esbuild scratch-check.ts --bundle --platform=node --format=esm --outfile=scratch-check.mjs && node scratch-check.mjs` e apague o `.mjs` depois.

- [ ] **Step 5: Apagar o scratch e fazer o build**

Run (de `cmd/hqreader/frontend`): `rm scratch-check.ts` e `npm run build`
Expected: build com sucesso (as libs ainda não estão no grafo do Vite, então isto só confirma que nada quebrou). `git status` não pode listar `scratch-check.*`.

- [ ] **Step 6: Commit**

```bash
git add cmd/hqreader/frontend/src/lib/computeSpread.ts cmd/hqreader/frontend/src/lib/viewTransform.ts
git commit -m "hqreader: logica pura de visao de pagina (spread) e matematica de zoom/pan"
```

---

### Task 3: Store — zoom, pan, modo de página e navegação por visão

**Files:**
- Modify: `cmd/hqreader/frontend/src/store/readerStore.ts`

**Interfaces:**
- Consumes: tudo de `computeSpread.ts` e `viewTransform.ts` (Task 2).
- Produces (consumido pelas Tasks 4–6): novos campos `zoom: number`, `pan: Point`, `metrics: { content: Size; viewport: Size }`, `pageMode: PageMode`, `pageAspects: PageAspects`; novas ações `goNext(): Promise<void>`, `goPrev(): Promise<void>`, `zoomBy(factor: number, cursor?: Point): void`, `panBy(dx: number, dy: number): void`, `resetView(): void`, `setMetrics(content: Size, viewport: Size): void`, `togglePageMode(): void`, `recordAspect(index: number, aspect: PageAspect): void`. `goToPage` passa a resetar zoom/pan. Os campos existentes (`bookID`, `pageCount`, `currentPage`, `loading`, `error`, `navError`, `loadCurrentBook`, `goToPage`, `clearNavError`) não mudam de nome nem de contrato.

- [ ] **Step 1: Verificar a precondição**

Run: `git log --oneline | grep -c "falha de navegacao"`
Expected: `1`. Se for `0`, PARE e reporte BLOCKED (ver "Precondition"). Leia `src/store/readerStore.ts` e confirme que ele tem `navError` e `clearNavError`.

- [ ] **Step 2: Imports** — em `readerStore.ts`, trocar

```ts
} from "../../bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service";
```

por

```ts
} from "../../bindings/github.com/DevMiguelDiniz/hq-reader-ultimate/internal/reader/service";
import {
  computeSpread,
  nextPageIndex,
  prevPageIndex,
  type PageAspect,
  type PageAspects,
  type PageMode,
} from "@/lib/computeSpread";
import { clampPan, zoomAt, type Point, type Size } from "@/lib/viewTransform";

const ORIGIN: Point = { x: 0, y: 0 };
const EMPTY_SIZE: Size = { width: 0, height: 0 };

interface ViewMetrics {
  content: Size;
  viewport: Size;
}
```

- [ ] **Step 3: Interface da store** — trocar

```ts
  loadCurrentBook: () => Promise<void>;
  goToPage: (index: number) => Promise<void>;
```

por

```ts
  zoom: number;
  pan: Point;
  metrics: ViewMetrics;
  pageMode: PageMode;
  pageAspects: PageAspects;
  loadCurrentBook: () => Promise<void>;
  goToPage: (index: number) => Promise<void>;
  goNext: () => Promise<void>;
  goPrev: () => Promise<void>;
  zoomBy: (factor: number, cursor?: Point) => void;
  panBy: (dx: number, dy: number) => void;
  resetView: () => void;
  setMetrics: (content: Size, viewport: Size) => void;
  togglePageMode: () => void;
  recordAspect: (index: number, aspect: PageAspect) => void;
```

- [ ] **Step 4: Estado inicial** — trocar

```ts
  loading: true,
  error: null,
```

por

```ts
  loading: true,
  error: null,
  zoom: 1,
  pan: ORIGIN,
  metrics: { content: EMPTY_SIZE, viewport: EMPTY_SIZE },
  pageMode: "single",
  pageAspects: {},
```

- [ ] **Step 5: Reset ao abrir livro** — dentro de `loadCurrentBook`, trocar

```ts
        currentPage: 0,
        loading: false,
      });
```

por

```ts
        currentPage: 0,
        loading: false,
        zoom: 1,
        pan: ORIGIN,
        pageAspects: {},
      });
```

- [ ] **Step 6: `goToPage` reseta a visão** — trocar

```ts
    set({ currentPage: index });
```

por

```ts
    set({ currentPage: index, zoom: 1, pan: ORIGIN });
```

- [ ] **Step 7: Novas ações** — trocar o `}));` final do arquivo por

```ts

  // Enquanto a visão está pending (alguma página do slot ainda sem proporção),
  // navegar ignoraria um possível spread ainda não medido e pularia uma
  // página; então goNext/goPrev não fazem nada até a visão ser decidida.
  goNext: async () => {
    const { currentPage, pageMode, pageAspects, pageCount, goToPage } = get();
    const view = computeSpread(currentPage, pageMode, pageAspects, pageCount);
    if (view.pending) {
      return;
    }
    const target = nextPageIndex(view, pageCount);
    if (target !== null) {
      await goToPage(target);
    }
  },

  goPrev: async () => {
    const { currentPage, pageMode, pageAspects, pageCount, goToPage } = get();
    const view = computeSpread(currentPage, pageMode, pageAspects, pageCount);
    if (view.pending) {
      return;
    }
    const target = prevPageIndex(view, pageMode, pageAspects, pageCount);
    if (target !== null) {
      await goToPage(target);
    }
  },

  // cursor é relativo ao centro do viewport; sem cursor, zoom no centro.
  zoomBy: (factor, cursor = ORIGIN) => {
    const { zoom, pan, metrics } = get();
    const next = zoomAt(
      zoom,
      pan,
      factor,
      cursor,
      metrics.content,
      metrics.viewport,
    );
    set({ zoom: next.zoom, pan: next.pan });
  },

  panBy: (dx, dy) => {
    const { zoom, pan, metrics } = get();
    set({
      pan: clampPan(
        { x: pan.x + dx, y: pan.y + dy },
        zoom,
        metrics.content,
        metrics.viewport,
      ),
    });
  },

  resetView: () => set({ zoom: 1, pan: ORIGIN }),

  // Chamado pelo PageView (ResizeObserver) a cada mudança de tamanho; reclampa
  // o pan para que redimensionar a janela com zoom ativo nunca deixe a imagem
  // fora do viewport.
  setMetrics: (content, viewport) => {
    const { metrics, zoom, pan } = get();
    const unchanged =
      metrics.content.width === content.width &&
      metrics.content.height === content.height &&
      metrics.viewport.width === viewport.width &&
      metrics.viewport.height === viewport.height;
    if (unchanged) {
      return;
    }
    set({
      metrics: { content, viewport },
      pan: clampPan(pan, zoom, content, viewport),
    });
  },

  togglePageMode: () =>
    set((state) => ({
      pageMode: state.pageMode === "single" ? "double" : "single",
      zoom: 1,
      pan: ORIGIN,
    })),

  recordAspect: (index, aspect) => {
    if (get().pageAspects[index] === aspect) {
      return;
    }
    set((state) => ({
      pageAspects: { ...state.pageAspects, [index]: aspect },
    }));
  },
}));
```

(O item anterior — `goToPage` ou `clearNavError` — já termina em vírgula; confira.)

- [ ] **Step 8: Smoke de bundle e build**

Run (de `cmd/hqreader/frontend`): `npx esbuild src/store/readerStore.ts --bundle --format=esm --tsconfig=tsconfig.json --log-level=warning --outfile=NUL`
Expected: sem erros nem warnings (confirma que os imports `@/lib/...` e os bindings resolvem). Depois `npm run build`: sucesso. Rode também o typecheck de melhor esforço (Global Constraints) e corrija erros novos em `readerStore.ts`.

- [ ] **Step 9: Commit**

```bash
git add cmd/hqreader/frontend/src/store/readerStore.ts
git commit -m "hqreader: store ganha zoom, pan, modo de pagina e navegacao por visao"
```

---

### Task 4: `PageView` e `Reader` — zoom/pan, par de páginas, placeholder de erro

**Files:**
- Create: `cmd/hqreader/frontend/src/components/PageView.tsx`
- Modify: `cmd/hqreader/frontend/src/components/Reader.tsx` (reescrita completa)

**Interfaces:**
- Consumes: store da Task 3; `computeSpread`, `viewLabel` da Task 2; `Button` de `@/components/ui/button`; `TriangleAlert` de `lucide-react`.
- Produces: `PageView` (default export, sem props) — renderiza a visão atual, mede viewport/conteúdo (`setMetrics`), trata roda (`zoomBy`) e arrasto (`panBy`), registra proporções (`recordAspect`) e mostra o placeholder de erro por página. `Reader` passa a exibir, na linha de controles: botão `Anterior`, contador `N / M` (ou `N-M / M`) num único nó de texto, botão `Proxima`, texto de zoom `100%` num único nó de texto, e o botão de modo (`Pagina unica`/`Pagina dupla`). `alt` das imagens vira `pagina N` (1-based).

- [ ] **Step 1: Criar `src/components/PageView.tsx`**

```tsx
import {
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { computeSpread } from "@/lib/computeSpread";
import { useReaderStore } from "@/store/readerStore";

interface PageSlotProps {
  bookID: string;
  index: number;
  wide: boolean;
  retry: number;
  failed: boolean;
  onLoad: (index: number, aspect: number) => void;
  onError: (index: number) => void;
  onRetry: (index: number) => void;
}

function PageSlot({
  bookID,
  index,
  wide,
  retry,
  failed,
  onLoad,
  onError,
  onRetry,
}: PageSlotProps) {
  if (failed) {
    return (
      <div
        role="alert"
        className="flex min-h-[40vh] min-w-[30vw] flex-col items-center justify-center gap-3 rounded-lg border border-neutral-700 p-6 text-neutral-300"
      >
        <TriangleAlert aria-hidden="true" className="size-8 text-amber-400" />
        <p>Não foi possível carregar esta página</p>
        <Button variant="secondary" onClick={() => onRetry(index)}>
          Tentar novamente
        </Button>
      </div>
    );
  }

  // O parâmetro ?r= só força um novo fetch no "Tentar novamente"; o handler
  // do backend usa apenas o path.
  const src = `/page/${bookID}/${index}${retry > 0 ? `?r=${retry}` : ""}`;
  return (
    <img
      src={src}
      alt={`pagina ${index + 1}`}
      draggable={false}
      className={
        wide ? "max-h-[80vh] max-w-[44vw]" : "max-h-[80vh] max-w-[90vw]"
      }
      onLoad={(e) => {
        const { naturalWidth, naturalHeight } = e.currentTarget;
        if (naturalHeight > 0) {
          onLoad(index, naturalWidth / naturalHeight);
        }
      }}
      onError={() => onError(index)}
    />
  );
}

export default function PageView() {
  const bookID = useReaderStore((s) => s.bookID);
  const currentPage = useReaderStore((s) => s.currentPage);
  const pageCount = useReaderStore((s) => s.pageCount);
  const pageMode = useReaderStore((s) => s.pageMode);
  const pageAspects = useReaderStore((s) => s.pageAspects);
  const zoom = useReaderStore((s) => s.zoom);
  const pan = useReaderStore((s) => s.pan);
  const recordAspect = useReaderStore((s) => s.recordAspect);
  const setMetrics = useReaderStore((s) => s.setMetrics);
  const zoomBy = useReaderStore((s) => s.zoomBy);
  const panBy = useReaderStore((s) => s.panBy);

  const viewportRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{ x: number; y: number } | null>(null);
  const [failed, setFailed] = useState<Record<number, boolean>>({});
  const [retries, setRetries] = useState<Record<number, number>>({});

  useEffect(() => {
    setFailed({});
    setRetries({});
  }, [bookID]);

  // Mede viewport e conteúdo (tamanho de layout, antes do transform) e
  // reporta à store, que reclampa o pan — cobre redimensionar a janela com
  // zoom ativo e a mudança de tamanho quando imagens terminam de carregar.
  useEffect(() => {
    const viewport = viewportRef.current;
    const content = contentRef.current;
    if (!viewport || !content) {
      return;
    }
    const measure = () =>
      setMetrics(
        { width: content.offsetWidth, height: content.offsetHeight },
        { width: viewport.clientWidth, height: viewport.clientHeight },
      );
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(viewport);
    observer.observe(content);
    return () => observer.disconnect();
  }, [setMetrics]);

  // Listener nativo não-passivo: o onWheel do React é passivo e não deixaria
  // chamar preventDefault.
  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) {
      return;
    }
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const rect = viewport.getBoundingClientRect();
      zoomBy(Math.exp(-e.deltaY * 0.0015), {
        x: e.clientX - (rect.left + rect.width / 2),
        y: e.clientY - (rect.top + rect.height / 2),
      });
    };
    viewport.addEventListener("wheel", onWheel, { passive: false });
    return () => viewport.removeEventListener("wheel", onWheel);
  }, [zoomBy]);

  if (!bookID) {
    return null;
  }

  const view = computeSpread(currentPage, pageMode, pageAspects, pageCount);
  const wide = view.indices.length > 1;

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) {
      return;
    }
    dragRef.current = { x: e.clientX, y: e.clientY };
    e.currentTarget.setPointerCapture(e.pointerId);
  };

  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    const last = dragRef.current;
    if (!last) {
      return;
    }
    panBy(e.clientX - last.x, e.clientY - last.y);
    dragRef.current = { x: e.clientX, y: e.clientY };
  };

  const endDrag = (e: ReactPointerEvent<HTMLDivElement>) => {
    dragRef.current = null;
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  };

  const handleLoad = (index: number, aspect: number) => {
    recordAspect(index, aspect);
  };

  const handleError = (index: number) => {
    setFailed((prev) => ({ ...prev, [index]: true }));
    recordAspect(index, "error");
  };

  const handleRetry = (index: number) => {
    setFailed((prev) => ({ ...prev, [index]: false }));
    setRetries((prev) => ({ ...prev, [index]: (prev[index] ?? 0) + 1 }));
  };

  return (
    <div
      ref={viewportRef}
      className="relative flex min-h-0 flex-1 touch-none items-center justify-center overflow-hidden select-none"
      style={{ cursor: zoom > 1 ? "grab" : "default" }}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
    >
      <div
        ref={contentRef}
        className={`flex items-center justify-center gap-2 will-change-transform ${view.pending ? "invisible" : ""}`}
        style={{
          transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
        }}
      >
        {view.indices.map((index) => (
          <PageSlot
            key={index}
            bookID={bookID}
            index={index}
            wide={wide}
            retry={retries[index] ?? 0}
            failed={failed[index] ?? false}
            onLoad={handleLoad}
            onError={handleError}
            onRetry={handleRetry}
          />
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Reescrever `src/components/Reader.tsx`**

```tsx
import { useEffect } from "react";
import PageView from "@/components/PageView";
import { Button } from "@/components/ui/button";
import { computeSpread, viewLabel } from "@/lib/computeSpread";
import { useReaderStore } from "@/store/readerStore";

export default function Reader() {
  const {
    bookID,
    pageCount,
    currentPage,
    loading,
    error,
    navError,
    zoom,
    pageMode,
    pageAspects,
    loadCurrentBook,
    goNext,
    goPrev,
    togglePageMode,
    clearNavError,
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

  const view = computeSpread(currentPage, pageMode, pageAspects, pageCount);
  const firstIndex = view.indices[0] ?? 0;
  const lastIndex = view.indices[view.indices.length - 1] ?? 0;

  return (
    <div className="flex h-screen flex-col bg-neutral-900 text-neutral-100">
      {navError && (
        <div className="fixed top-4 left-1/2 z-10 flex -translate-x-1/2 items-center gap-3 rounded-md bg-red-900/90 px-4 py-2 text-sm text-red-100 shadow-lg">
          <span>Erro ao navegar: {navError}</span>
          <button
            type="button"
            onClick={clearNavError}
            aria-label="Dispensar aviso"
            className="text-red-200 hover:text-white"
          >
            ×
          </button>
        </div>
      )}
      <PageView />
      <div className="flex items-center justify-center gap-4 py-3">
        <Button
          variant="secondary"
          onClick={() => goPrev()}
          disabled={firstIndex <= 0}
        >
          Anterior
        </Button>
        <span>{`${viewLabel(view)} / ${pageCount}`}</span>
        <Button
          variant="secondary"
          onClick={() => goNext()}
          disabled={lastIndex >= pageCount - 1}
        >
          Proxima
        </Button>
        <span>{`${Math.round(zoom * 100)}%`}</span>
        <Button variant="secondary" onClick={togglePageMode}>
          {pageMode === "single" ? "Pagina unica" : "Pagina dupla"}
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Build e typecheck**

Run (de `cmd/hqreader/frontend`): `npm run build`
Expected: sucesso. Rode o typecheck de melhor esforço e corrija erros novos em `PageView.tsx`/`Reader.tsx` (atenção: `lucide-react` precisa exportar `TriangleAlert` — a versão instalada, 1.47.0, exporta; se algum dia não exportar, use o equivalente da versão instalada, conferindo com `grep -c TriangleAlert node_modules/lucide-react/dist/lucide-react.d.ts`).

- [ ] **Step 4: Verificação manual com evidência real**

Gere o CBZ (Task 1, Step 2) e rode, de `cmd/hqreader`: `$env:HQREADER_BOOK = "$env:TEMP\hqreader-interactions.cbz"; wails3 dev` (tente de novo se não conectar). Com a janela real do `hqreader.exe` aberta, capture screenshots reais (padrão `PrintWindow` usado na Task 4 da Etapa 3b; salve no diretório do relatório da task) e confirme, descrevendo o que **viu**: (a) a página 1 renderiza como imagem e a linha de controles mostra `1 / 10`, `100%` e `Pagina unica`; (b) um passo de roda do mouse sobre a página (`mouse_event` com `MOUSEEVENTF_WHEEL`) muda o texto de zoom para um valor maior que `100%` e a imagem cresce; (c) clicar `Pagina dupla` e avançar até a visão `6-7 / 10` mostra a página 6 como imagem e, no lugar da 7, o bloco de aviso com o botão `Tentar novamente`; (d) avançar para o contador `8-9 / 10` mostra duas imagens lado a lado. Pare o `wails3 dev` ao final. Não commite os screenshots.

- [ ] **Step 5: Commit**

```bash
git add cmd/hqreader/frontend/src/components/PageView.tsx cmd/hqreader/frontend/src/components/Reader.tsx
git commit -m "hqreader: PageView com zoom/pan, par de paginas e placeholder de erro por pagina"
```

---

### Task 5: Motion — fade na troca de página

**Files:**
- Modify: `cmd/hqreader/frontend/package.json` e `package-lock.json` (via `npm install motion`)
- Modify: `cmd/hqreader/frontend/src/components/PageView.tsx`

**Interfaces:**
- Consumes: `PageView` da Task 4.
- Produces: a mesma API do `PageView`, agora com fade de ~150ms (75ms saindo + 75ms entrando) entre visões, e troca instantânea com `prefers-reduced-motion`.

- [ ] **Step 1: Instalar a dependência**

Run (de `cmd/hqreader/frontend`): `npm install motion`
Expected: `motion` entra em `dependencies`. Confira que `AnimatePresence`, `motion` e `useReducedMotion` são exportados por `motion/react`: `node -e "import('motion/react').then(m => console.log(typeof m.AnimatePresence, typeof m.motion, typeof m.useReducedMotion))"` deve imprimir `object object function` (ou `function`/`object` equivalentes — o ponto é que nenhum seja `undefined`). Se algum for `undefined`, PARE e reporte BLOCKED com a saída.

- [ ] **Step 2: Imports e constante no `PageView.tsx`** — trocar

```tsx
import { TriangleAlert } from "lucide-react";
```

por

```tsx
import { TriangleAlert } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
```

e trocar

```tsx
interface PageSlotProps {
```

por

```tsx
// Duração de cada metade (saída e entrada) do fade: ~150ms no total.
const FADE_HALF_DURATION = 0.075;

interface PageSlotProps {
```

- [ ] **Step 3: Hook de movimento reduzido** — dentro de `PageView`, trocar

```tsx
  const viewportRef = useRef<HTMLDivElement>(null);
```

por

```tsx
  const reduceMotion = useReducedMotion();
  const viewportRef = useRef<HTMLDivElement>(null);
```

- [ ] **Step 4: Envolver as páginas no `AnimatePresence`** — trocar

```tsx
      <div
        ref={contentRef}
        className={`flex items-center justify-center gap-2 will-change-transform ${view.pending ? "invisible" : ""}`}
        style={{
          transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
        }}
      >
        {view.indices.map((index) => (
          <PageSlot
            key={index}
            bookID={bookID}
            index={index}
            wide={wide}
            retry={retries[index] ?? 0}
            failed={failed[index] ?? false}
            onLoad={handleLoad}
            onError={handleError}
            onRetry={handleRetry}
          />
        ))}
      </div>
```

por

```tsx
      <div
        ref={contentRef}
        className={`will-change-transform ${view.pending ? "invisible" : ""}`}
        style={{
          transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
        }}
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={view.indices.join("-")}
            className="flex items-center justify-center gap-2"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: reduceMotion ? 0 : FADE_HALF_DURATION }}
          >
            {view.indices.map((index) => (
              <PageSlot
                key={index}
                bookID={bookID}
                index={index}
                wide={wide}
                retry={retries[index] ?? 0}
                failed={failed[index] ?? false}
                onLoad={handleLoad}
                onError={handleError}
                onRetry={handleRetry}
              />
            ))}
          </motion.div>
        </AnimatePresence>
      </div>
```

- [ ] **Step 5: Build e verificação manual**

Run: `npm run build` (sucesso) e o typecheck de melhor esforço. Depois `wails3 dev` com o CBZ da Task 1 (como na Task 4, Step 4) e confirme com screenshot/leitura da árvore real que, 600ms depois de clicar `Proxima` várias vezes seguidas, existe **exatamente uma** imagem na visão (nenhuma imagem antiga presa pela saída do fade) e o contador está correto. O fade em si (75ms+75ms) não é observável por UI Automation — registre isso no relatório como limitação, não como verificado.

- [ ] **Step 6: Commit**

```bash
git add cmd/hqreader/frontend/package.json cmd/hqreader/frontend/package-lock.json cmd/hqreader/frontend/src/components/PageView.tsx
git commit -m "hqreader: Motion no fade de troca de pagina (respeita prefers-reduced-motion)"
```

---

### Task 6: Atalhos de teclado

**Files:**
- Create: `cmd/hqreader/frontend/src/hooks/useReaderShortcuts.ts`
- Modify: `cmd/hqreader/frontend/src/components/Reader.tsx`

**Interfaces:**
- Consumes: ações `goNext`, `goPrev`, `zoomBy`, `resetView`, `togglePageMode` e campo `bookID` da store (Task 3); `ZOOM_KEY_STEP` de `viewTransform.ts` (Task 2).
- Produces: `useReaderShortcuts(): void` — instala um único listener `keydown` na `window` enquanto o `Reader` está montado.

- [ ] **Step 1: Criar `src/hooks/useReaderShortcuts.ts`**

```ts
import { useEffect } from "react";
import { ZOOM_KEY_STEP } from "@/lib/viewTransform";
import { useReaderStore } from "@/store/readerStore";

export function useReaderShortcuts() {
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // Combinações com Ctrl/Alt/Meta pertencem ao sistema, não ao leitor.
      if (e.ctrlKey || e.altKey || e.metaKey) {
        return;
      }
      const target = e.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable)
      ) {
        return;
      }

      const store = useReaderStore.getState();
      if (!store.bookID) {
        return;
      }

      switch (e.key) {
        case "ArrowLeft":
        case "a":
        case "A":
          void store.goPrev();
          break;
        case "ArrowRight":
        case "d":
        case "D":
          void store.goNext();
          break;
        case " ":
          // Tira o foco de um botão focado: senão o clique de teclado dele
          // (disparado no keyup do Espaço) avançaria uma segunda página.
          (document.activeElement as HTMLElement | null)?.blur();
          void store.goNext();
          break;
        case "+":
        case "=":
          store.zoomBy(ZOOM_KEY_STEP);
          break;
        case "-":
          store.zoomBy(1 / ZOOM_KEY_STEP);
          break;
        case "0":
          store.resetView();
          break;
        case "m":
        case "M":
          store.togglePageMode();
          break;
        default:
          return;
      }
      e.preventDefault();
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
}
```

- [ ] **Step 2: Usar o hook em `Reader.tsx`** — adicionar o import e a chamada **antes** de qualquer `return` antecipado (regra dos hooks). Trocar

```tsx
import { computeSpread, viewLabel } from "@/lib/computeSpread";
```

por

```tsx
import { useReaderShortcuts } from "@/hooks/useReaderShortcuts";
import { computeSpread, viewLabel } from "@/lib/computeSpread";
```

e trocar

```tsx
  // loadCurrentBook é chamado uma vez ao montar. Sob React StrictMode (dev),
```

por

```tsx
  useReaderShortcuts();

  // loadCurrentBook é chamado uma vez ao montar. Sob React StrictMode (dev),
```

- [ ] **Step 3: Build, typecheck e verificação manual**

Run: `npm run build` e o typecheck de melhor esforço. Depois `wails3 dev` com o CBZ da Task 1 e, com a janela real focada (clique uma vez no centro da página antes), envie teclas (`System.Windows.Forms.SendKeys`) e confirme pela árvore de UI Automation real: `{RIGHT}` → contador `2 / 10`; `{LEFT}` → `1 / 10`; `{+}` → texto de zoom `115%`; `0` → `100%`; `m` → botão `Pagina dupla` aparece; `^{RIGHT}` (Ctrl+→) **não** muda o contador. Relate o que foi observado.

- [ ] **Step 4: Commit**

```bash
git add cmd/hqreader/frontend/src/hooks/useReaderShortcuts.ts cmd/hqreader/frontend/src/components/Reader.tsx
git commit -m "hqreader: atalhos de teclado do leitor"
```

---

### Task 7: Verificação de ponta a ponta (dev e produção)

**Files:**
- Create: `cmd/hqreader/verify-reader-interactions.ps1`
- Create: `cmd/hqreader/verify-reader-interactions-dev-output.txt`
- Create: `cmd/hqreader/verify-reader-interactions-prod-output.txt`
- Modify: `cmd/hqreader/MANUAL_VERIFICATION.md` (só acrescentar uma seção)
- Modify: `cmd/hqreader/verify-navigation-react.ps1` (só um aviso de "superado" no cabeçalho)

**Interfaces:**
- Consumes: o app completo (Tasks 1–6) e o CBZ de 10 páginas da Task 1 (visões esperadas no modo duplo: `[0] [1,2] [3] [4] [5,6] [7,8] [9]`; página índice 6 corrompida).

Padrão de rigor deste projeto (precedentes reais: Etapa 2 Task 6 e Etapa 3b Task 1 voltaram para correção por evidência incompleta): o script precisa ser **real, completo e re-executável**, e a saída bruta precisa ser a **saída real, não editada**, de duas execuções separadas (dev e produção). Não resuma nem narre o que a saída "mostraria".

- [ ] **Step 1: Criar `cmd/hqreader/verify-reader-interactions.ps1`**

```powershell
<#
Etapa 3c — verificacao E2E das interacoes do leitor (zoom, pan, modo de
pagina unica/dupla, atalhos de teclado, placeholder de erro por pagina,
reset ao trocar de pagina, redimensionar com zoom ativo).

Nao e codigo de producao. PowerShell descartavel que dirige a janela REAL do
hqreader.exe com Windows UI Automation (System.Windows.Automation) e com
entrada real de mouse/teclado (user32 SetCursorPos/mouse_event e
System.Windows.Forms.SendKeys). Roda igual contra `wails3 dev` e contra o
binario de `wails3 build` (bin/hqreader.exe).

NAO MEXA NO MOUSE NEM NO TECLADO enquanto o script roda: ele move o cursor
de verdade e envia teclas para a janela em primeiro plano.

Pre-requisito: o livro aberto e o CBZ de 10 paginas gerado por
`go run make-verify-cbz.go <saida.cbz>` (de cmd/hqreader), apontado por
HQREADER_BOOK. Indices 0-based: 0 capa, 4 spread (paisagem), 6 corrompida.
Visoes no modo duplo: [0] [1,2] [3] [4] [5,6] [7,8] [9].

O que o script le na arvore de UI Automation (cada dado e UM no de texto):
  - contador:  Text com Name "N / M" ou "N-M / M"
  - zoom:      Text com Name "NNN%"
  - modo:      botao com Name "Pagina unica" ou "Pagina dupla"
  - paginas:   controles Image com Name "pagina N" (1-based)
  - erro:      botao "Tentar novamente" e um Text contendo "carregar esta"
  - posicao:   BoundingRectangle do Image (reflete transform/pan)

Como rodar (dev):
  1. cd cmd/hqreader
  2. go run make-verify-cbz.go $env:TEMP\hqreader-interactions.cbz
  3. $env:HQREADER_BOOK = "$env:TEMP\hqreader-interactions.cbz"
  4. wails3 dev   (em segundo plano; espere a janela "hqreader")
  5. powershell -NoProfile -ExecutionPolicy Bypass -File verify-reader-interactions.ps1 -ProcessName hqreader

Como rodar (producao): wails3 build; mesmo HQREADER_BOOK; Start-Process
.\bin\hqreader.exe; mesmo comando do passo 5.

Sai com 0 se todas as checagens passam, 1 caso contrario.
#>

param(
    [string]$ProcessName = "hqreader"
)

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class HqReaderInteractionsWin32 {
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, uint dx, uint dy, int dwData, UIntPtr dwExtraInfo);
}
"@

$MOUSEEVENTF_LEFTDOWN = 0x0002
$MOUSEEVENTF_LEFTUP = 0x0004
$MOUSEEVENTF_WHEEL = 0x0800

$global:pass = 0
$global:fail = 0

function Check($label, $actual, $expected) {
    if ($actual -eq $expected) {
        Write-Host "PASS: $label -> '$actual'"
        $global:pass++
    } else {
        Write-Host "FAIL: $label -> got '$actual', expected '$expected'"
        $global:fail++
    }
}

function CheckTrue($label, $condition, $detail) {
    if ($condition) {
        Write-Host "PASS: $label ($detail)"
        $global:pass++
    } else {
        Write-Host "FAIL: $label ($detail)"
        $global:fail++
    }
}

# ---- UI Automation helpers ------------------------------------------------

function Find-ByName($parent, $name, $controlType) {
    $nameCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::NameProperty, $name)
    if ($controlType) {
        $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, $controlType)
        $cond = New-Object System.Windows.Automation.AndCondition($nameCond, $typeCond)
    } else {
        $cond = $nameCond
    }
    return $parent.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $cond)
}

function Test-ButtonExists($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    return [bool]$btn
}

function Invoke-ByName($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $pattern = $btn.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
    $pattern.Invoke()
}

function Get-ButtonEnabled($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    return $btn.Current.IsEnabled
}

function Get-TextNames($parent) {
    $typeCond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Text)
    $all = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $typeCond)
    $names = @()
    foreach ($el in $all) { $names += $el.Current.Name }
    return $names
}

function Get-TextMatching($parent, $pattern) {
    $deadline = (Get-Date).AddSeconds(10)
    while ((Get-Date) -lt $deadline) {
        foreach ($n in (Get-TextNames $parent)) {
            if ($n -match $pattern) { return $n }
        }
        Start-Sleep -Milliseconds 300
    }
    return $null
}

function Get-Counter($parent) { return Get-TextMatching $parent '^\d+(-\d+)? / \d+$' }

function Get-ZoomPercent($parent) {
    $t = Get-TextMatching $parent '^\d+%$'
    if ($t) { return [int]($t -replace '%', '') }
    return $null
}

function Get-ImageNames($parent) {
    $cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ControlTypeProperty, [System.Windows.Automation.ControlType]::Image)
    $imgs = $parent.FindAll([System.Windows.Automation.TreeScope]::Descendants, $cond)
    $names = @()
    foreach ($i in $imgs) { $names += $i.Current.Name }
    return (($names | Sort-Object) -join ',')
}

function Get-ImageRect($parent, $name) {
    $img = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Image)
    if (-not $img) { throw "image '$name' not found" }
    return $img.Current.BoundingRectangle
}

# Polling checks: the UI settles asynchronously (store update, image load,
# fade), so wait up to 5s for the expected value before declaring failure.
function Check-Counter($label, $expected) {
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-Counter $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-Counter $script:win
    }
    Check $label $actual $expected
}

function Check-Zoom($label, $expected) {
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-ZoomPercent $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-ZoomPercent $script:win
    }
    Check $label $actual $expected
}

function Check-Images($label, $expectedNames) {
    $expected = (($expectedNames | Sort-Object) -join ',')
    $deadline = (Get-Date).AddSeconds(5)
    $actual = Get-ImageNames $script:win
    while ($actual -ne $expected -and (Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 250
        $actual = Get-ImageNames $script:win
    }
    Check $label $actual $expected
}

# ---- Real input helpers ---------------------------------------------------

function Move-Cursor($x, $y) {
    [HqReaderInteractionsWin32]::SetCursorPos([int]$x, [int]$y) | Out-Null
}

function Click-At($x, $y) {
    Move-Cursor $x $y
    Start-Sleep -Milliseconds 80
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 60
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTUP, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 400
}

function Click-Button($parent, $name) {
    $btn = Find-ByName $parent $name ([System.Windows.Automation.ControlType]::Button)
    if (-not $btn) { throw "button '$name' not found" }
    $r = $btn.Current.BoundingRectangle
    Click-At ($r.X + $r.Width / 2) ($r.Y + $r.Height / 2)
}

# notches > 0 = wheel up (zoom in), < 0 = wheel down (zoom out).
function Send-Wheel($x, $y, $notches) {
    Move-Cursor $x $y
    Start-Sleep -Milliseconds 80
    $delta = if ($notches -ge 0) { 120 } else { -120 }
    for ($i = 0; $i -lt [math]::Abs($notches); $i++) {
        [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_WHEEL, 0, 0, $delta, [UIntPtr]::Zero)
        Start-Sleep -Milliseconds 120
    }
    Start-Sleep -Milliseconds 300
}

function Send-Drag($x, $y, $dx, $dy) {
    Move-Cursor $x $y
    Start-Sleep -Milliseconds 100
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTDOWN, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 100
    $steps = 10
    for ($i = 1; $i -le $steps; $i++) {
        Move-Cursor ($x + $dx * $i / $steps) ($y + $dy * $i / $steps)
        Start-Sleep -Milliseconds 30
    }
    Start-Sleep -Milliseconds 100
    [HqReaderInteractionsWin32]::mouse_event($MOUSEEVENTF_LEFTUP, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 400
}

function Send-Keys($keys) {
    [System.Windows.Forms.SendKeys]::SendWait($keys)
    Start-Sleep -Milliseconds 450
}

function Get-PageCenter() {
    $r = $script:win.Current.BoundingRectangle
    return @{ X = ($r.X + $r.Width / 2); Y = ($r.Y + $r.Height * 0.45) }
}

function Test-RectsOverlap($a, $b) {
    return ($a.Right -gt $b.X) -and ($a.X -lt $b.Right) -and ($a.Bottom -gt $b.Y) -and ($a.Y -lt $b.Bottom)
}

# ---- Locate the window ----------------------------------------------------

Write-Host "=== Locating $ProcessName.exe process ==="
$proc = Get-Process $ProcessName -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $proc) { throw "$ProcessName.exe not running" }
Write-Host "PID: $($proc.Id)"

$root = [System.Windows.Automation.AutomationElement]::RootElement
$cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ProcessIdProperty, $proc.Id)
$deadline = (Get-Date).AddSeconds(30)
$win = $null
while ((Get-Date) -lt $deadline) {
    $windows = $root.FindAll([System.Windows.Automation.TreeScope]::Children, $cond)
    if ($windows.Count -gt 0) { $win = $windows[0]; break }
    Start-Sleep -Milliseconds 500
}
if (-not $win) { throw "no top-level window appeared for PID $($proc.Id) within 30s" }
Write-Host "Found window: $($win.Current.Name)"

Write-Host "`n=== Restoring/foregrounding window (WebView2 exposes no UI Automation descendants while minimized/hidden) ==="
if ($proc.MainWindowHandle -ne [IntPtr]::Zero) {
    [HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
    [HqReaderInteractionsWin32]::SetForegroundWindow($proc.MainWindowHandle) | Out-Null
}
Start-Sleep -Milliseconds 800
Write-Host "BoundingRectangle after restore: $($win.Current.BoundingRectangle)"

$c = Get-PageCenter
Click-At $c.X $c.Y   # give the WebView keyboard focus (no pan happens at 100%)

# ---- 6.1 Initial state ----------------------------------------------------
Write-Host "`n=== 6.1: initial state ==="
Check-Counter "initial counter" "1 / 10"
Check-Zoom "initial zoom" 100
CheckTrue "mode button reads 'Pagina unica'" (Test-ButtonExists $win 'Pagina unica') "button present"
Check-Images "initial image" @('pagina 1')

# ---- 6.2 Keyboard navigation (single mode) --------------------------------
Write-Host "`n=== 6.2: keyboard navigation, single mode ==="
Send-Keys "{RIGHT}"; Check-Counter "Right arrow" "2 / 10"
Send-Keys "{LEFT}";  Check-Counter "Left arrow" "1 / 10"
Send-Keys "d";       Check-Counter "key d" "2 / 10"
Send-Keys "a";       Check-Counter "key a" "1 / 10"
Send-Keys " ";       Check-Counter "Space" "2 / 10"
Send-Keys "{LEFT}";  Check-Counter "back to start" "1 / 10"
Send-Keys "{RIGHT 5}"; Check-Counter "burst of 5x Right" "6 / 10"
Send-Keys "{LEFT 5}";  Check-Counter "burst of 5x Left" "1 / 10"
Send-Keys "^{RIGHT}";  Check-Counter "Ctrl+Right must NOT navigate" "1 / 10"
Send-Keys "{LEFT}";    Check-Counter "Left at first page stays" "1 / 10"
Click-Button $win 'Proxima'
Check-Counter "real mouse click on Proxima" "2 / 10"
Send-Keys " "
Check-Counter "Space after clicking a button advances exactly one page" "3 / 10"
Send-Keys "{LEFT 2}"; Check-Counter "back to start again" "1 / 10"

# ---- 6.3 Keyboard zoom -----------------------------------------------------
Write-Host "`n=== 6.3: keyboard zoom ==="
Send-Keys "{+}"; Check-Zoom "key + once" 115
Send-Keys "-";   Check-Zoom "key - once" 100
Send-Keys "{+}{+}"; Check-Zoom "key + twice" 132
Send-Keys "0";   Check-Zoom "key 0 resets" 100
Send-Keys "=";   Check-Zoom "key = zooms in" 115
Send-Keys "0";   Check-Zoom "key 0 resets again" 100

# ---- 6.4 Wheel zoom and limits ---------------------------------------------
Write-Host "`n=== 6.4: wheel zoom and limits ==="
$c = Get-PageCenter
Send-Wheel $c.X $c.Y 3
$z = Get-ZoomPercent $win
CheckTrue "3 notches up zoom in (130..200)" (($z -ge 130) -and ($z -le 200)) "zoom=$z%"
Send-Wheel $c.X $c.Y -3
$z = Get-ZoomPercent $win
CheckTrue "3 notches down return near 100 (95..105)" (($z -ge 95) -and ($z -le 105)) "zoom=$z%"
Send-Keys "0"
Send-Wheel $c.X $c.Y 14
Check-Zoom "14 notches up clamp at 400" 400
Send-Keys "0"
Send-Wheel $c.X $c.Y -10
Check-Zoom "10 notches down clamp at 50" 50
Send-Keys "0"
Check-Zoom "reset after limits" 100

# ---- 6.5 Pan, clamp, page-change reset, resize while zoomed ----------------
Write-Host "`n=== 6.5: pan, clamp, reset on page change, resize while zoomed ==="
Send-Wheel $c.X $c.Y 4
$zoomed = Get-ZoomPercent $win
CheckTrue "zoomed in for pan test (>150)" ($zoomed -gt 150) "zoom=$zoomed%"
$r0 = Get-ImageRect $win 'pagina 1'
Write-Host "Image rect before drag: $r0"

Send-Drag $c.X $c.Y 0 50
$r1 = Get-ImageRect $win 'pagina 1'
$dy1 = $r1.Y - $r0.Y
CheckTrue "vertical drag +50 moves image down ~50 (25..75)" (($dy1 -ge 25) -and ($dy1 -le 75)) "deltaY=$dy1"

Send-Drag $c.X $c.Y 0 600
$r2 = Get-ImageRect $win 'pagina 1'
$dy2 = $r2.Y - $r0.Y
CheckTrue "huge vertical drag is clamped (>50 and <500)" (($dy2 -gt 50) -and ($dy2 -lt 500)) "deltaY=$dy2"

$rx0 = Get-ImageRect $win 'pagina 1'
Send-Drag $c.X $c.Y 100 0
$rx1 = Get-ImageRect $win 'pagina 1'
$dx = $rx1.X - $rx0.X
CheckTrue "horizontal drag is clamped when content narrower than viewport (|dx|<=2)" ([math]::Abs($dx) -le 2) "deltaX=$dx"

Write-Host "--- resize while zoomed: maximize ---"
[HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 3) | Out-Null   # SW_MAXIMIZE
Start-Sleep -Milliseconds 1000
$winRect = $win.Current.BoundingRectangle
$imgRect = Get-ImageRect $win 'pagina 1'
CheckTrue "image still intersects window after maximize" (Test-RectsOverlap $imgRect $winRect) "image=$imgRect window=$winRect"
Check-Zoom "zoom unchanged by maximize" $zoomed
[HqReaderInteractionsWin32]::ShowWindow($proc.MainWindowHandle, 9) | Out-Null   # SW_RESTORE
Start-Sleep -Milliseconds 1000
$winRect = $win.Current.BoundingRectangle
$imgRect = Get-ImageRect $win 'pagina 1'
CheckTrue "image still intersects window after restore" (Test-RectsOverlap $imgRect $winRect) "image=$imgRect window=$winRect"

Write-Host "--- reset on page change ---"
Invoke-ByName $win 'Proxima'
Check-Counter "Proxima while zoomed" "2 / 10"
Check-Zoom "page change resets zoom to 100" 100
Invoke-ByName $win 'Anterior'
Check-Counter "Anterior back to first page" "1 / 10"

# ---- 6.6 Double page mode --------------------------------------------------
Write-Host "`n=== 6.6: double page mode ==="
Send-Keys "m"
CheckTrue "mode button reads 'Pagina dupla'" (Test-ButtonExists $win 'Pagina dupla') "button present"
Check-Counter "cover alone" "1 / 10"
Check-Images "cover alone images" @('pagina 1')
Send-Keys "{RIGHT}"; Check-Counter "pair 2-3" "2-3 / 10"
Check-Images "pair 2-3 images" @('pagina 2', 'pagina 3')
Send-Keys "{RIGHT}"; Check-Counter "page 4 alone (neighbor of spread)" "4 / 10"
Check-Images "page 4 images" @('pagina 4')
Send-Keys "{RIGHT}"; Check-Counter "page 5 alone (spread)" "5 / 10"
Check-Images "spread image" @('pagina 5')
Send-Keys "{RIGHT}"; Check-Counter "pair 6-7 (7 is corrupted)" "6-7 / 10"
Check-Images "only page 6 renders as image" @('pagina 6')
CheckTrue "page 7 shows the retry button" (Test-ButtonExists $win 'Tentar novamente') "button present"
Send-Keys "{RIGHT}"; Check-Counter "pair 8-9" "8-9 / 10"
Check-Images "pair 8-9 images" @('pagina 8', 'pagina 9')
Send-Keys "{RIGHT}"; Check-Counter "last page alone" "10 / 10"
Check-Images "last page image" @('pagina 10')
Send-Keys "{RIGHT}"; Check-Counter "Right at last stays" "10 / 10"
CheckTrue "Proxima disabled at the last view" (-not (Get-ButtonEnabled $win 'Proxima')) "IsEnabled=False"
Send-Keys "{LEFT}"; Check-Counter "back to pair 8-9" "8-9 / 10"
Send-Keys "{LEFT}"; Check-Counter "back to pair 6-7" "6-7 / 10"
Send-Keys "{LEFT}"; Check-Counter "back to spread" "5 / 10"
Send-Keys "{LEFT}"; Check-Counter "back to page 4" "4 / 10"
Send-Keys "{LEFT}"; Check-Counter "back to pair 2-3" "2-3 / 10"
Send-Keys "{LEFT}"; Check-Counter "back to cover" "1 / 10"
CheckTrue "Anterior disabled at the first view" (-not (Get-ButtonEnabled $win 'Anterior')) "IsEnabled=False"

Write-Host "--- toggle back to single from a pair ---"
# Every page was visited above, so every proportion is known and no view is
# pending: the burst cannot be dropped by the pending guard (Decision 8).
Send-Keys "{RIGHT 5}"; Check-Counter "to pair 8-9 again" "8-9 / 10"
Send-Keys "m"
CheckTrue "mode button reads 'Pagina unica'" (Test-ButtonExists $win 'Pagina unica') "button present"
Check-Counter "single mode keeps first page of the pair" "8 / 10"
Check-Zoom "toggling mode resets zoom" 100
Send-Keys "{LEFT 7}"; Check-Counter "back to first page" "1 / 10"

# ---- 6.7 Per-page error placeholder (single mode) --------------------------
Write-Host "`n=== 6.7: per-page error placeholder, single mode ==="
Send-Keys "{RIGHT 6}"
Check-Counter "navigate to the corrupted page" "7 / 10"
CheckTrue "retry button present on corrupted page" (Test-ButtonExists $win 'Tentar novamente') "button present"
$msg = (Get-TextNames $win) | Where-Object { $_ -like '*carregar esta*' } | Select-Object -First 1
CheckTrue "error message text present" ([bool]$msg) "text='$msg'"
Check-Images "no image for the corrupted page" @()
CheckTrue "Anterior still enabled on the error page" (Get-ButtonEnabled $win 'Anterior') "IsEnabled=True"
CheckTrue "Proxima still enabled on the error page" (Get-ButtonEnabled $win 'Proxima') "IsEnabled=True"
Invoke-ByName $win 'Tentar novamente'
Start-Sleep -Milliseconds 800
CheckTrue "still a placeholder after retry (page is really corrupt)" (Test-ButtonExists $win 'Tentar novamente') "button present"
Check-Counter "counter unchanged after retry" "7 / 10"
Send-Keys "{RIGHT}"; Check-Counter "navigate past the error page" "8 / 10"
CheckTrue "placeholder gone on a good page" (-not (Test-ButtonExists $win 'Tentar novamente')) "button absent"
Check-Images "good page image" @('pagina 8')

Write-Host "`n=== SUMMARY: $global:pass passed, $global:fail failed ==="
if ($global:fail -gt 0) { exit 1 }
exit 0
```

- [ ] **Step 2: Validar a sintaxe do script (sem rodar o app)**

Run (PowerShell): `$errors = $null; [System.Management.Automation.Language.Parser]::ParseFile("$PWD\cmd\hqreader\verify-reader-interactions.ps1", [ref]$null, [ref]$errors) | Out-Null; $errors.Count`
Expected: `0`.

- [ ] **Step 3: Inspecionar a árvore real antes de confiar nas suposições do script**

Gere o CBZ, rode `wails3 dev` com `HQREADER_BOOK` (ver o cabeçalho do script) e, com a janela real aberta, despeje a árvore de UI Automation (todos os descendentes com `ControlType`, `Name`, `BoundingRectangle`). Confirme **três suposições do script** com o que realmente aparece: (a) o contador (`1 / 10`), o zoom (`100%`) e a mensagem de erro são nós `Text` individuais com esses `Name`; (b) o `<img>` aparece como `Image` com `Name='pagina 1'` e seu `BoundingRectangle` muda quando há zoom/pan; (c) o botão de modo aparece como `Button` com `Name='Pagina unica'`. Se alguma suposição **não** valer (por exemplo, o `BoundingRectangle` do `Image` não refletir o `transform`), PARE e reporte BLOCKED com o dump real — não adapte o script por conta própria nem desligue a checagem afetada.

- [ ] **Step 4: Rodar em modo dev e capturar a saída bruta**

```powershell
cd cmd/hqreader
go run make-verify-cbz.go $env:TEMP\hqreader-interactions.cbz
$env:HQREADER_BOOK = "$env:TEMP\hqreader-interactions.cbz"
wails3 dev
```
(Tente de novo se não conectar — ver Global Constraints. Garanta que não há outro `hqreader.exe` rodando.) Quando a janela abrir, **sem tocar no mouse/teclado**, em outro terminal:

```powershell
cd cmd/hqreader
.\verify-reader-interactions.ps1 *> verify-reader-interactions-dev-output.txt
```
Pare o `wails3 dev` depois. Se algum `FAIL` aparecer, **não edite a saída**: investigue a causa. Se for bug no app, corrija o app na task dona do código (e reporte); se for suposição errada do script, corrija o script e **rode de novo do zero**, gerando a saída outra vez. O arquivo final precisa ser a saída bruta de uma execução completa com `0 failed`.

- [ ] **Step 5: Repetir no build de produção**

```bash
wails3 build
```
Rode `bin/hqreader.exe` com o mesmo `HQREADER_BOOK` e, sem tocar no mouse/teclado:

```powershell
.\verify-reader-interactions.ps1 *> verify-reader-interactions-prod-output.txt
```
Isto exercita o caminho de assets embutidos (`AssetFileServerFS`), diferente do proxy do Vite do modo dev. Os dois `-output.txt` devem ser de execuções separadas (PIDs diferentes), ambos terminando em `=== SUMMARY: N passed, 0 failed ===`.

- [ ] **Step 6: Acrescentar a seção em `MANUAL_VERIFICATION.md`**

Acrescentar ao **final** de `cmd/hqreader/MANUAL_VERIFICATION.md` (não substituir nada do conteúdo existente), preenchendo os campos entre `<...>` com valores copiados dos dois arquivos de saída:

```markdown

## Etapa 3c — zoom/pan, modo duplo, atalhos, placeholder de erro, Motion

**Data:** <data da execução>
**Cenário:** CBZ sintético de 10 páginas (`make-verify-cbz.go`): capa, retratos,
um spread (índice 4) e uma página corrompida (índice 6, bytes que não são PNG).

Script real: `verify-reader-interactions.ps1`. Saída bruta:
`verify-reader-interactions-dev-output.txt` (wails3 dev, PID <pid dev>),
`verify-reader-interactions-prod-output.txt` (wails3 build, PID <pid prod>).
Resultado: `<linha SUMMARY do dev>` e `<linha SUMMARY do prod>`.

- [ ] Navegação por teclado (setas, A/D, Espaço, rajada, Ctrl+→ ignorado, Espaço após clique em botão avança uma página)
- [ ] Zoom por teclado (+, =, -, 0) e por roda, com limites 50% e 400%
- [ ] Arrastar move a imagem (eixo vertical), pan limitado ao viewport
- [ ] Redimensionar (maximizar/restaurar) com zoom ativo mantém a imagem visível
- [ ] Trocar de página reseta o zoom para 100%
- [ ] Modo duplo: capa sozinha, pares, spread sozinho, vizinho do spread sozinho, última página sozinha, ida e volta
- [ ] Alternar o modo mantém a primeira página da visão e reseta o zoom
- [ ] Placeholder de erro (modo único e metade de um par no modo duplo), navegação continua habilitada, "Tentar novamente" funciona sem derrubar a leitura
- [ ] Mesmo comportamento em produção (build embutido)

(Marque cada item com `[x]` somente se as linhas `PASS` correspondentes estão nos dois arquivos de saída.)

**Observações:** <desvios do esperado, ou "nenhum">

**Limitações desta verificação:** a transição de Motion (fade de ~150ms) não é
observável por UI Automation; o que se verifica é que, depois de navegar, só
as imagens esperadas existem na árvore (nenhuma imagem antiga presa pela saída
do fade). O comportamento em WebKitGTK/Linux não foi validado (fora de escopo
da etapa, spec §10.6). O script observa a UI e a posição do `<img>`; nunca o
estado Go diretamente.
```

- [ ] **Step 7: Marcar o script da Etapa 3b como superado**

Em `cmd/hqreader/verify-navigation-react.ps1`, trocar a linha

```
Not production code. Throwaway PowerShell using Windows UI Automation
```

por

```
SUPERADO pela Etapa 3c: a UI agora tem mais nos de texto (zoom "100%") alem
do contador, e Get-CounterText abaixo concatena TODOS os nos Text e exige
que o resultado seja so "N / M" — portanto este script nao passa mais contra
o app atual. Mantido como registro historico; use verify-reader-interactions.ps1.

Not production code. Throwaway PowerShell using Windows UI Automation
```

- [ ] **Step 8: Commit**

```bash
git add cmd/hqreader/verify-reader-interactions.ps1 cmd/hqreader/verify-reader-interactions-dev-output.txt cmd/hqreader/verify-reader-interactions-prod-output.txt cmd/hqreader/MANUAL_VERIFICATION.md cmd/hqreader/verify-navigation-react.ps1
git commit -m "hqreader: verificacao de ponta a ponta das interacoes do leitor (dev e producao)"
```

---

## Self-Review

**Cobertura da spec §10:** 10.1 zoom/pan por transform + roda centrada no cursor + limites 50–400% + reset ao trocar de página → Tasks 2 (`zoomAt`/`clampPan`), 3 (`goToPage` reseta), 4 (roda/arrasto); página única/dupla em memória, capa sozinha, spread sozinho, navegação por par → Tasks 2 (`computeSpread`), 3 (`togglePageMode`, `goNext/goPrev`), 4 (botão e contador); placeholder de erro com ícone, texto, "Tentar novamente", metade do par → Task 4 (`PageSlot`); Motion só na troca de página com `prefers-reduced-motion` → Task 5. 10.2 atalhos → Task 6. 10.3 estrutura de arquivos (`readerStore.ts`, `lib/computeSpread.ts`, `components/PageView.tsx`, `hooks/useReaderShortcuts.ts`, `Reader.tsx`) → Tasks 3, 2, 4, 6, 4; `lib/viewTransform.ts` é um arquivo a mais (a matemática de zoom/pan cabe melhor fora da store, para o scratch check poder exercitá-la). 10.4 proporção via `onLoad` e espera antes de renderizar o par → `pending` (Tasks 2, 4). 10.5 verificação só por UI Automation, CBZ com capa/retratos/spread/total ímpar... → Task 7 (o CBZ tem 10 páginas, total par; o caso "última página sozinha" é coberto porque o último slot `(9,10)` é incompleto — visão `[9]`). 10.6 fora de escopo respeitado: nenhuma task toca backend, persistência, webtoon, RTL ou Linux.

**Placeholders:** nenhum "TBD"/"TODO". Os únicos campos `<...>` estão no texto do `MANUAL_VERIFICATION.md` da Task 7 Step 6, que o implementador preenche com valores lidos dos arquivos de saída reais (datas, PIDs, linha SUMMARY) — dados que só existem depois da execução, com instrução explícita de onde copiá-los.

**Consistência de tipos:** `ReaderView`, `PageMode`, `PageAspect(s)` definidos na Task 2 são importados com os mesmos nomes na Task 3. `Point`/`Size`/`zoomAt`/`clampPan`/`ZOOM_KEY_STEP` da Task 2 são usados com a mesma assinatura nas Tasks 3 e 6. As ações da Task 3 (`goNext`, `goPrev`, `zoomBy(factor, cursor?)`, `panBy(dx, dy)`, `resetView`, `setMetrics(content, viewport)`, `togglePageMode`, `recordAspect(index, aspect)`) são chamadas com esses nomes e argumentos em `PageView` (Task 4), `Reader` (Task 4) e `useReaderShortcuts` (Task 6). O `alt` `pagina N` e os nomes de botão/texto que o script da Task 7 procura (`Anterior`, `Proxima`, `Pagina unica`, `Pagina dupla`, `Tentar novamente`, contador `N / M`, zoom `NNN%`) batem exatamente com o JSX da Task 4. O trecho que a Task 5 substitui em `PageView.tsx` é idêntico, caractere por caractere, ao produzido pela Task 4.

**Review Focus:** as cinco linhas têm teste na task dona: 1 e 2 no scratch check (Task 2), 3, 4 e 5 nos passos 6.5, 6.2 e 6.7 do script (Task 7) com o código correspondente nas Tasks 4 e 6.

**Risco de merge:** `readerStore.ts` e `Reader.tsx` também são tocados pela branch `claude/sweet-bardeen-eb792f` (ver Precondition). A Task 3 usa edições ancoradas em linhas presentes antes e depois daquele merge, e a Task 4 reescreve o `Reader.tsx` já incluindo o banner `navError` dela. O placeholder de erro por página (`PageSlot`, estado local `failed`/`retries` no `PageView`) é um mecanismo independente dos campos `error`/`navError` da store.
