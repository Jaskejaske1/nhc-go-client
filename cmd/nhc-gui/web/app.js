const roomsElement = document.querySelector("#rooms");
const errorElement = document.querySelector("#error");
const connectionText = document.querySelector("#connection-text");
const activeCount = document.querySelector("#active-count");
const updatedAt = document.querySelector("#updated-at");

let state = { actions: [], locations: [] };

function locationName(id) {
  const location = state.locations.find((item) => item.id === id);
  return location && location.name ? location.name : "default";
}

function escapeText(value) {
  return String(value).replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#039;"
  }[character]));
}

function render() {
  const groups = new Map();
  state.actions.forEach((action) => {
    const name = locationName(action.location);
    if (!groups.has(name)) groups.set(name, []);
    groups.get(name).push(action);
  });
  activeCount.textContent = `${state.actions.filter((action) => action.value1 > 0).length} active actions`;
  updatedAt.textContent = `Updated ${new Date().toLocaleTimeString()}`;
  roomsElement.innerHTML = [...groups.entries()].sort().map(([name, actions]) => `
    <article class="room">
      <div class="room-head"><h3>${escapeText(name)}</h3><span class="room-count">${actions.length} actions</span></div>
      ${actions.map(renderAction).join("")}
    </article>`).join("");
  roomsElement.querySelectorAll("button[data-action-id]").forEach((element) => {
    element.addEventListener("click", () => sendAction(Number(element.dataset.actionId), element.dataset.value));
  });
  roomsElement.querySelectorAll("input[data-action-id]").forEach((element) => {
    element.addEventListener("change", () => sendAction(Number(element.dataset.actionId), element.value));
  });
}

function renderAction(action) {
  const on = action.value1 > 0;
  const isLight = action.type === "LIGHT";
  const controls = isLight
    ? `<input class="slider" type="range" min="0" max="100" value="${action.value1}" data-action-id="${action.id}" aria-label="${escapeText(action.name)} brightness">`
    : "";
  return `<div class="device">
    <div><p class="device-name">${escapeText(action.name)}</p><span class="device-meta">${action.type} / ${on ? `${action.value1}%` : "OFF"}</span></div>
    <div class="device-actions">${controls}<button class="${on ? "active" : ""}" data-action-id="${action.id}" data-value="${on ? 0 : 100}">${on ? "Off" : "On"}</button></div>
  </div>`;
}

let loading = false;

async function load() {
  if (loading) return;
  loading = true;
  try {
    const response = await fetch("/api/status");
    if (!response.ok) throw new Error(await response.text());
    state = await response.json();
    errorElement.hidden = true;
    connectionText.textContent = "Connected";
    render();
  } catch (error) {
    connectionText.textContent = "Unavailable";
    errorElement.textContent = error.message;
    errorElement.hidden = false;
  } finally {
    loading = false;
  }
}

async function sendAction(id, value) {
  try {
    const payload = Number(value) === 0 ? { id, on: false } : { id, brightness: Number(value) };
    const response = await fetch("/api/actions", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
    if (!response.ok) throw new Error(await response.text());
    await load();
  } catch (error) {
    errorElement.textContent = error.message;
    errorElement.hidden = false;
  }
}

document.querySelector("#refresh").addEventListener("click", load);
load();
setInterval(load, 5000);