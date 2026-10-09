/** From the Parley template (MIT). */
export function init(reduced) {
  // ============ FAQ: animate open/close of <details> ============
  const faqItems = Array.from(document.querySelectorAll(".faq__item"));
  faqItems.forEach((item) => {
    const summary = item.querySelector("summary");
    const answer = item.querySelector(".faq__answer");
    if (!summary || !answer) return;

    let anim = null;

    const collapse = () => {
      const startH = answer.offsetHeight;
      if (anim) anim.cancel();
      anim = answer.animate(
        [{ height: `${startH}px`, opacity: 1 }, { height: "0px", opacity: 0 }],
        { duration: reduced ? 0 : 280, easing: "cubic-bezier(0.2, 0, 0, 1)" }
      );
      anim.onfinish = () => {
        item.removeAttribute("open");
        answer.style.height = "";
        anim = null;
      };
    };

    const expand = () => {
      item.setAttribute("open", "");
      const endH = answer.scrollHeight;
      if (anim) anim.cancel();
      anim = answer.animate(
        [{ height: "0px", opacity: 0 }, { height: `${endH}px`, opacity: 1 }],
        { duration: reduced ? 0 : 320, easing: "cubic-bezier(0.32, 0.72, 0, 1)" }
      );
      anim.onfinish = () => {
        answer.style.height = "";
        anim = null;
      };
    };

    summary.addEventListener("click", (e) => {
      e.preventDefault();
      if (item.hasAttribute("open")) {
        collapse();
      } else {
        // close any other open item first
        faqItems.forEach((other) => {
          if (other !== item && other.hasAttribute("open")) {
            other.removeAttribute("open");
          }
        });
        expand();
      }
    });
  });
}
