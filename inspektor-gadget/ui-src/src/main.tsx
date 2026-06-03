import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DensityProvider, ThemeProvider } from "@flanksource/clicky-ui";
import { createRoot } from "react-dom/client";
import { App } from "./components/App";
import "./styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

const queryClient = new QueryClient();
createRoot(root).render(
  <ThemeProvider>
    <DensityProvider>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </DensityProvider>
  </ThemeProvider>
);
