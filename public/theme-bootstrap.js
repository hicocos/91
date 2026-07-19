(function () {
  try {
    var theme = localStorage.getItem("video-site:theme");
    if (theme === "pink" || theme === "dark" || theme === "sky") {
      document.documentElement.setAttribute("data-theme", theme);
    } else {
      document.documentElement.setAttribute("data-theme", "dark");
    }
  } catch (_error) {
    document.documentElement.setAttribute("data-theme", "dark");
  }
})();
