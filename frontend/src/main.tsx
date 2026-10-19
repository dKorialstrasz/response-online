import { createRoot } from "react-dom/client";

import { App } from "./App";
import "./styles.css";

const root = document.getElementById("root");
if (root === null) {
  throw new Error("Не найден корневой элемент приложения");
}

createRoot(root).render(<App />);
