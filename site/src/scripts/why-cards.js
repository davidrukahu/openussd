/** From the Parley template (MIT). */
export function init(reduced) {
  // ============ WHY-CARDS: sticky hover (exact framer Benefit cards) ============
  // Each card opens on mouseenter and STAYS open until another card is
  // hovered - no reset on pointer leave (framer wires onMouseEnter to
  // SET_VARIANT with no reverse handler).
  const whyRow = document.getElementById("why-cards");
  if (whyRow) {
    const wcards = Array.from(whyRow.querySelectorAll(".wcard"));
    const activate = (card) => {
      if (card.classList.contains("is-active")) return;
      wcards.forEach((c) => c.classList.toggle("is-active", c === card));
    };
    wcards.forEach((card) => {
      card.addEventListener("pointerenter", () => activate(card));
      card.addEventListener("focus", () => activate(card));
    });
  }
}
