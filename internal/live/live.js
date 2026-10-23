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

// The browser reconnects on its own while the lab runs, so a closed stream is
// the only one that will not come back.
stream.onerror = () => {
  connection.textContent = stream.readyState === EventSource.CLOSED
    ? "the lab no longer answers"
    : "reconnecting to the lab";
};
