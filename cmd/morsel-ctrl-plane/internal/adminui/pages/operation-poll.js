// Polls one or more async operations and reflects their status in the DOM.
// Each .op-row element carries data-status-url (JSON endpoint returning
// {status, progress, error}) and starts in the "pending" state. Polling stops
// once every row reaches "complete" or "failed". If everything succeeded the
// page auto-redirects to data-return; if anything failed it stays put and
// shows a manual "Back" link instead, so the operator has time to read the
// error rather than being bounced away from it.
(function () {
  var container = document.getElementById("op-poll");
  if (!container) {
    return;
  }
  var returnURL = container.getAttribute("data-return");
  var rows = Array.prototype.slice.call(container.querySelectorAll(".op-row"));
  var pollIntervalMs = 1500;

  function setRowState(row, state, message) {
    row.setAttribute("data-state", state);
    var badge = row.querySelector(".op-row-badge");
    var msg = row.querySelector(".op-row-message");
    if (badge) {
      badge.className = "badge op-row-badge " + badgeClass(state);
      badge.textContent = badgeText(state);
    }
    if (msg) {
      msg.textContent = message || "";
    }
  }

  function badgeClass(state) {
    switch (state) {
      case "complete":
        return "badge--green";
      case "failed":
        return "badge--red";
      default:
        return "badge--yellow";
    }
  }

  function badgeText(state) {
    switch (state) {
      case "complete":
        return "✓ Done";
      case "failed":
        return "✕ Failed";
      default:
        return "↻ In progress";
    }
  }

  function allSettled() {
    return rows.every(function (row) {
      var s = row.getAttribute("data-state");
      return s === "complete" || s === "failed";
    });
  }

  function failedCount() {
    return rows.filter(function (row) {
      return row.getAttribute("data-state") === "failed";
    }).length;
  }

  function summarize() {
    var failed = failedCount();
    if (failed === 0) {
      return rows.length === 1 ? "Done." : "All " + rows.length + " operations completed.";
    }
    return failed + " of " + rows.length + " operation(s) failed.";
  }

  function poll(row) {
    var url = row.getAttribute("data-status-url");
    fetch(url, { headers: { Accept: "application/json" } })
      .then(function (resp) {
        if (!resp.ok) {
          throw new Error("status check failed: " + resp.status);
        }
        return resp.json();
      })
      .then(function (data) {
        if (data.status === "complete") {
          setRowState(row, "complete", data.progress || "");
        } else if (data.status === "failed") {
          var msg = (data.error && data.error.message) || data.progress || "operation failed";
          setRowState(row, "failed", msg);
        } else {
          setRowState(row, "pending", data.progress || "");
        }
      })
      .catch(function (err) {
        // Transient network hiccups shouldn't stop polling; just retry next tick.
        setRowState(row, "pending", "checking...");
      });
  }

  function tick() {
    rows.forEach(function (row) {
      if (row.getAttribute("data-state") === "pending") {
        poll(row);
      }
    });
    if (allSettled()) {
      var summary = document.getElementById("op-poll-summary");
      if (summary) {
        summary.textContent = summarize();
      }
      if (failedCount() > 0) {
        // Something failed — stay on the page so the operator can read the
        // error message; don't whisk them away from it automatically.
        var back = document.getElementById("op-poll-return");
        if (back) {
          var base = back.getAttribute("href");
          var sep = base.indexOf("?") >= 0 ? "&" : "?";
          back.href = base + sep + "flash_error=1&flash=" + encodeURIComponent(summarize());
          back.style.display = "";
        }
        return;
      }
      setTimeout(function () {
        if (returnURL) {
          window.location.href = returnURL + (returnURL.indexOf("?") >= 0 ? "&" : "?") + "flash=" + encodeURIComponent(summarize());
        }
      }, 800);
      return;
    }
    setTimeout(tick, pollIntervalMs);
  }

  tick();
})();
