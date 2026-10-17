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
        } else {
          // Sem proporcao valida a visao dupla ficaria pendente para sempre
          // (goNext/goPrev ignoram visoes pendentes); trata como pagina falha.
          onError(index);
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
