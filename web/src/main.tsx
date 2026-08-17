import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import "./styles/app.css";
import "./styles/environment.css";
import "./styles/navigation.css";
import "./styles/workspace-control.css";
import "./styles/branch-selection.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
