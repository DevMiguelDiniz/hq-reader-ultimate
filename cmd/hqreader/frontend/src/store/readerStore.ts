import { create } from "zustand";
import {
  Current,
  SetCurrentPage,
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

interface ReaderState {
  bookID: string | null;
  pageCount: number;
  currentPage: number;
  loading: boolean;
  // error é fatal: falha ao abrir o livro, sem UI de leitura pra mostrar.
  error: string | null;
  // navError é uma falha ao navegar dentro de um livro já aberto — o
  // reader continua funcional, só avisa e permite dispensar o aviso.
  navError: string | null;
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
  clearNavError: () => void;
}

export const useReaderStore = create<ReaderState>((set, get) => ({
  bookID: null,
  pageCount: 0,
  currentPage: 0,
  loading: true,
  error: null,
  zoom: 1,
  pan: ORIGIN,
  metrics: { content: EMPTY_SIZE, viewport: EMPTY_SIZE },
  pageMode: "single",
  pageAspects: {},
  navError: null,

  // loadCurrentBook consulta o último livro aberto (Current()) e registra a
  // página inicial (0) no backend via SetCurrentPage — o estado da página
  // sempre vive no servidor, nunca só neste store (§3.2 do spec).
  loadCurrentBook: async () => {
    set({ loading: true, error: null, navError: null });
    try {
      const result = await Current();
      set({
        bookID: result.BookID,
        pageCount: result.PageCount,
        currentPage: 0,
        loading: false,
        zoom: 1,
        pan: ORIGIN,
        pageAspects: {},
      });
      await SetCurrentPage(result.BookID, 0);
    } catch (err) {
      set({ loading: false, error: String(err) });
    }
  },

  // goToPage atualiza o estado local (pra UI responder na hora) e chama
  // SetCurrentPage em seguida — a fonte da verdade é o backend. Se a
  // chamada falhar, volta currentPage pro valor confirmado anteriormente
  // em vez de deixar a UI mostrar uma página que o backend nunca aceitou.
  goToPage: async (index: number) => {
    const { bookID, pageCount, currentPage: previousPage } = get();
    if (!bookID || index < 0 || index >= pageCount) {
      return;
    }
    set({ currentPage: index, zoom: 1, pan: ORIGIN });
    try {
      await SetCurrentPage(bookID, index);
    } catch (err) {
      set({ currentPage: previousPage, navError: String(err) });
    }
  },

  clearNavError: () => set({ navError: null }),

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
