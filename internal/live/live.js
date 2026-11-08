"use strict";

// The board arrives rendered and escaped by the lab, so it is placed as markup.
const board = document.getElementById("board");
const connection = document.getElementById("connection");
const stream = new EventSource("/board");

stream.onopen = () => {
  connection.textContent = "";
};

stream.onmessage = (event) => {
  board.innerHTML = event.data;
};

// The innermost box carrying a Failure takes the click, so a Zone takes one on
// its name line and its margin around its Nodes, and a Node one outside its
// Pods. A Pending Pod's box carries none, so a click on it posts nothing.
board.addEventListener("click", (event) => {
  const clicked = event.target.closest("[data-failure]");
  if (clicked === null) {
    return;
  }
  fetch(clicked.dataset.failure, { method: "POST" });
});

// The browser reconnects on its own while the lab runs, so a closed stream is
// the only one that will not come back.
stream.onerror = () => {
  connection.textContent = stream.readyState === EventSource.CLOSED
    ? "the lab no longer answers"
    : "reconnecting to the lab";
};
