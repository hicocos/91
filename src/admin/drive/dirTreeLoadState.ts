export type DirTreeLoadState = "idle" | "loading" | "loaded" | "error";
export type DirTreeLoadEvent = "start" | "resolve" | "reject" | "effect" | "retry";

export function nextDirTreeLoadState(
  state: DirTreeLoadState,
  event: DirTreeLoadEvent
): DirTreeLoadState {
  if (event === "retry" && state === "error") return "idle";
  if (event === "start" && state === "idle") return "loading";
  if (event === "resolve" && state === "loading") return "loaded";
  if (event === "reject" && state === "loading") return "error";
  return state;
}
