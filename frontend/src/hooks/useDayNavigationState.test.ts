import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useDayNavigationState } from "./useDayNavigationState";

describe("useDayNavigationState", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("allows calendar stepping when the selected day has no stories", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-05-13T12:00:00Z"));

    const { result } = renderHook(() =>
      useDayNavigationState({
        dayBuckets: [{ day: "2026-05-11", story_count: 5 }],
        day: "2026-05-12",
        from: "",
        to: "",
      }),
    );

    expect(result.current.dayNav.navigatorDay).toBe("2026-05-12");
    expect(result.current.dayNav.currentIndex).toBe(-1);
    expect(result.current.dayNav.canGoOlder).toBe(true);
    expect(result.current.dayNav.canGoNewer).toBe(true);
  });
});
