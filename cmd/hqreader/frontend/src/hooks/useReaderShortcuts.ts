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
