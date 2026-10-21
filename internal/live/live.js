"use strict";

// The board arrives rendered and escaped by the lab, so it is placed as markup.
const board = document.getElementById("board");
const refreshMs = Number(board.dataset.refreshMs);

async function refresh() {
  try {
    const answered = await fetch("/board", { cache: "no-store" });
    if (!answered.ok) {
      board.textContent = "the lab answered " + answered.status;
      return;
    }
    board.innerHTML = await answered.text();
  } catch (stopped) {
    board.textContent = "the lab no longer answers: " + stopped;
  }
}

refresh();
setInterval(refresh, refreshMs);
