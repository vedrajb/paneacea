import App from "./App.svelte";
import { mount } from "svelte";
import "@xterm/xterm/css/xterm.css";
import "./app.css";
const app = mount(App, { target: document.getElementById("app")! });
export default app;
