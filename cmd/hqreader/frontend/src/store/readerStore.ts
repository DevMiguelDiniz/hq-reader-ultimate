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
  // error é fatal: falha ao abrir o livro, sem UI de leitura pra mostrar.
  error: string | null;
  // navError é uma falha ao navegar dentro de um livro já aberto — o
  // reader continua funcional, só avisa e permite dispensar o aviso.
  navError: string | null;
  loadCurrentBook: () => Promise<void>;
  goToPage: (index: number) => Promise<void>;
  clearNavError: () => void;
}

export const useReaderStore = create<ReaderState>((set, get) => ({
  bookID: null,
  pageCount: 0,
  currentPage: 0,
  loading: true,
  error: null,
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
    set({ currentPage: index });
    try {
      await SetCurrentPage(bookID, index);
    } catch (err) {
      set({ currentPage: previousPage, navError: String(err) });
    }
  },

  clearNavError: () => set({ navError: null }),
}));
