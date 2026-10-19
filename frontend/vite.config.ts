import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        ws: true,
        configure(proxy) {
          proxy.on("proxyReq", (proxyRequest, request) => {
            // Browsers omit Origin on same-origin GET requests. The local Vite
            // boundary restores the exact browser origin before forwarding so
            // the backend can keep its strict Origin allowlist.
            if (!request.headers.origin && request.headers.host) {
              proxyRequest.setHeader(
                "Origin",
                `http://${request.headers.host}`,
              );
            }
          });
        },
      },
      "/healthz": "http://127.0.0.1:8080",
      "/readyz": "http://127.0.0.1:8080",
    },
  },
  preview: {
    host: "127.0.0.1",
    port: 4173,
    strictPort: true,
  },
});
