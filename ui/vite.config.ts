import { fileURLToPath, URL } from "node:url";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite-plus";

export default defineConfig({
  plugins: [tanstackRouter({ target: "react", autoCodeSplitting: true }), react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  fmt: {
    ignorePatterns: [
      "dist/**",
      "src/routeTree.gen.ts",
      "src/api/schema.d.ts",
      "src/components/ui/**",
      "src/hooks/use-mobile.ts",
    ],
  },
  lint: {
    ignorePatterns: [
      "dist/**",
      "src/routeTree.gen.ts",
      "src/api/schema.d.ts",
      "src/components/ui/**",
      "src/hooks/use-mobile.ts",
    ],
    plugins: ["react"],
    jsPlugins: [{ name: "vite-plus", specifier: "vite-plus/oxlint-plugin" }],
    rules: {
      "vite-plus/prefer-vite-plus-imports": "error",
      "react/rules-of-hooks": "error",
      "react/exhaustive-deps": "error",
    },
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  test: {
    include: ["src/**/*.test.ts"],
    environment: "node",
  },
});
