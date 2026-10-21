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
