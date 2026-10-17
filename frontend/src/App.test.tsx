import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App } from "./App";

describe("App", () => {
  it("marks the build as local and does not claim verified rules", () => {
    const html = renderToStaticMarkup(<App />);

    expect(html).toContain("Локальный прототип M1");
    expect(html).toContain("Проверяем локальный backend");
    expect(html).toContain("не входят в M1");
  });
});
