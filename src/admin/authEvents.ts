type UnauthorizedListener = () => void;

const listeners = new Set<UnauthorizedListener>();

export function emitUnauthorized(): void {
  for (const listener of listeners) {
    listener();
  }
}

export function subscribeUnauthorized(listener: UnauthorizedListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
