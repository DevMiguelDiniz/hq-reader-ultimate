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
          variant="secondary"
          onClick={() => goToPage(currentPage - 1)}
          disabled={currentPage <= 0}
        >
          Anterior
        </Button>
        <span>
          {currentPage + 1} / {pageCount}
        </span>
        <Button
          variant="secondary"
          onClick={() => goToPage(currentPage + 1)}
          disabled={currentPage >= pageCount - 1}
        >
          Proxima
        </Button>
      </div>
    </div>
  );
}
