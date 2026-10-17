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
