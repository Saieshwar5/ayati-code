import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import "./styles/app.css";
import "./styles/environment.css";
import "./styles/navigation.css";
import "./styles/workspace-control.css";
import "./styles/workspace-panel.css";
import "./styles/branch-selection.css";
import "./styles/agent-studio.css";
import "./styles/compute-environments.css";
import "./styles/workspace-creation.css";
import "./styles/workspace-preparation.css";
import "./styles/composer-context.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
