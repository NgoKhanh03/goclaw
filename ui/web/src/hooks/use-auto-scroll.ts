import { useEffect, useRef, useCallback } from "react";

/**
 * Auto-scroll to bottom of a container when content changes.
 * Only auto-scrolls if user is near the bottom (within threshold).
 * Call `forceScrollToBottom()` to always scroll regardless of position
 * (e.g. when the user sends a new message).
 */
export function useAutoScroll<T extends HTMLElement>(
  deps: unknown[],
  threshold = 100,
) {
  const ref = useRef<T>(null);
  const isNearBottom = useRef(true);

  const checkScroll = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    const { scrollTop, scrollHeight, clientHeight } = el;
    isNearBottom.current = scrollHeight - scrollTop - clientHeight < threshold;
  }, [threshold]);

  const scrollToBottom = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, []);

  /** Scroll xuống cuối bất kể vị trí hiện tại, và đặt lại flag để
   *  các event tiếp theo (streaming) cũng tự scroll. */
  const forceScrollToBottom = useCallback(() => {
    isNearBottom.current = true;
    const el = ref.current;
    if (!el) return;
    // Dùng requestAnimationFrame để đảm bảo DOM đã render xong
    requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
    });
  }, []);

  useEffect(() => {
    if (isNearBottom.current) {
      scrollToBottom();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { ref, onScroll: checkScroll, scrollToBottom, forceScrollToBottom };
}
