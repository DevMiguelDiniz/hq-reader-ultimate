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
