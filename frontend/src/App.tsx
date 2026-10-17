import { useEffect, useState } from "react";

type BuildInfo = {
  commit: string;
  builtAt: string;
  protocolVersion: string;
  contentVersion: string;
  rulesVersion: string;
  engineVersion: string;
};

type BackendState =
  | { status: "loading" }
  | { status: "available"; info: BuildInfo }
  | { status: "unavailable" };

export function App() {
  const [backend, setBackend] = useState<BackendState>({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    void fetch("/api/v1/build", { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        return (await response.json()) as BuildInfo;
      })
      .then((info) => setBackend({ status: "available", info }))
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          setBackend({ status: "unavailable" });
        }
      });

    return () => controller.abort();
  }, []);

  return (
    <main>
      <section className="panel" aria-labelledby="page-title">
        <p className="eyebrow">Локальный прототип M1</p>
        <h1 id="page-title">Response Online</h1>
        <p className="lead">
          Инженерный scaffold готов. Игровые правила и способности карт появятся
          только после проверки официальных источников в связанных задачах.
        </p>

        <div className="status" role="status" aria-live="polite">
          {backend.status === "loading" && "Проверяем локальный backend…"}
          {backend.status === "unavailable" &&
            "Backend недоступен. Запустите его по инструкции локальной разработки."}
          {backend.status === "available" && (
            <>
              <strong>Backend подключён.</strong>
              <span>Commit: {backend.info.commit}</span>
              <span>Контент: {backend.info.contentVersion}</span>
              <span>Правила: {backend.info.rulesVersion}</span>
            </>
          )}
        </div>

        <p className="scope-note">
          Публичный deployment, GamePix и заявление о правах на материалы не
          входят в M1.
        </p>
      </section>
    </main>
  );
}
