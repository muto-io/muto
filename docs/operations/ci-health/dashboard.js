/* CI/CD health dashboard: renders data.json (see scripts/ci-health/collect.py)
 * into #ci-health-root. Uses textContent only; no HTML is built from data. */
(function () {
  "use strict";

  var STALE_HOURS = 48;

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
  }

  function isNum(value) {
    return typeof value === "number" && isFinite(value);
  }

  function pct(rate) {
    return isNum(rate) ? (rate * 100).toFixed(1) + "%" : "n/a";
  }

  function hrs(value) {
    return isNum(value) ? value.toFixed(1) + " h" : "n/a";
  }

  function tile(label, value, note) {
    var node = el("div", "ci-health-tile");
    node.appendChild(el("div", "ci-health-tile-label", label));
    node.appendChild(el("div", "ci-health-tile-value", value));
    node.appendChild(el("div", "ci-health-tile-note", note || ""));
    return node;
  }

  function table(headers, rows) {
    var t = el("table");
    var head = el("tr");
    headers.forEach(function (h) { head.appendChild(el("th", null, h)); });
    t.appendChild(el("thead")).appendChild(head);
    var body = el("tbody");
    rows.forEach(function (cells) {
      var tr = el("tr");
      cells.forEach(function (cell) { tr.appendChild(el("td", null, cell)); });
      body.appendChild(tr);
    });
    t.appendChild(body);
    return t;
  }

  function section(title, content) {
    var wrap = el("section", "ci-health-section");
    wrap.appendChild(el("h2", null, title));
    wrap.appendChild(content);
    return wrap;
  }

  function unavailable(text) {
    return el("p", "ci-health-unavailable", text || "Unavailable");
  }

  function ratesTable(firstHeader, rates) {
    var rows = Object.keys(rates).map(function (name) {
      var r = rates[name];
      return [name, pct(r.rate), String(r.success), String(r.failed)];
    });
    return table([firstHeader, "Success rate", "Passed", "Failed"], rows);
  }

  function aggregate(rates) {
    var success = 0, failed = 0;
    Object.keys(rates).forEach(function (name) {
      success += rates[name].success;
      failed += rates[name].failed;
    });
    var total = success + failed;
    return { rate: total ? success / total : null, total: total };
  }

  function render(root, data) {
    root.textContent = "";
    var errors = data.errors || {};

    var generated = new Date(data.generated_at);
    var ageHours = (Date.now() - generated.getTime()) / 36e5;
    var meta = "Last updated " + generated.toUTCString() + " · window: last " +
      (data.window ? data.window.days : "?") + " days";
    root.appendChild(el("p", "ci-health-meta", meta));
    if (isNum(ageHours) && ageHours > STALE_HOURS) {
      root.appendChild(el("p", "ci-health-stale",
        "This data is more than " + STALE_HOURS + " hours old; the daily refresh may be failing."));
    }

    var tiles = el("div", "ci-health-tiles");
    var wf = data.workflows;
    tiles.appendChild(wf
      ? tile("Workflow success rate", pct(wf.overall.rate), wf.overall.total + " runs")
      : tile("Workflow success rate", "n/a", errors.workflows || "unavailable"));

    var tj = data.test_jobs;
    if (tj) {
      var agg = aggregate(tj);
      tiles.appendChild(tile("Test pass rate", pct(agg.rate), agg.total + " job runs"));
    } else {
      tiles.appendChild(tile("Test pass rate", "n/a", errors.test_jobs || "unavailable"));
    }

    var dep = data.dependabot;
    tiles.appendChild(dep
      ? tile("Dependabot PRs", dep.opened + " opened / " + dep.merged + " merged",
          "median time to merge " + hrs(dep.median_hours_to_merge))
      : tile("Dependabot PRs", "n/a", errors.dependabot || "unavailable"));

    var rev = data.review;
    var first = rev ? rev.first_review_hours : null;
    if (rev) {
      tiles.appendChild(tile("Time to first review",
        first ? hrs(first.median) : "n/a",
        first ? "p90 " + hrs(first.p90) + " · n = " + first.n : "n = " + rev.sample + " PRs"));
    } else {
      tiles.appendChild(tile("Time to first review", "n/a", errors.review || "unavailable"));
    }
    root.appendChild(tiles);

    root.appendChild(section("Workflows",
      wf ? ratesTable("Workflow", wf.by_workflow) : unavailable(errors.workflows)));
    root.appendChild(section("Test jobs",
      tj ? ratesTable("Job", tj) : unavailable(errors.test_jobs)));

    if (dep) {
      var weekly = dep.weekly.map(function (w) {
        return [w.week, String(w.opened), String(w.merged)];
      });
      root.appendChild(section("Dependabot per week",
        weekly.length ? table(["Week", "Opened", "Merged"], weekly)
                      : unavailable("No Dependabot activity in this window")));
    } else {
      root.appendChild(section("Dependabot per week", unavailable(errors.dependabot)));
    }

    if (rev) {
      var m = rev.merge_hours;
      root.appendChild(section("Review turnaround",
        table(["Measure", "Median", "p90", "n"], [
          ["Open to first review",
            first ? hrs(first.median) : "n/a", first ? hrs(first.p90) : "n/a", first ? String(first.n) : "0"],
          ["Open to merge",
            m ? hrs(m.median) : "n/a", m ? hrs(m.p90) : "n/a", m ? String(m.n) : "0"]
        ])));
    } else {
      root.appendChild(section("Review turnaround", unavailable(errors.review)));
    }
  }

  function load(root) {
    root.textContent = "Loading…";
    fetch(root.getAttribute("data-src") || "data.json", { cache: "no-store" })
      .then(function (response) {
        if (!response.ok) throw new Error("HTTP " + response.status);
        return response.json();
      })
      .then(function (data) { render(root, data); })
      .catch(function () {
        root.textContent = "";
        root.appendChild(unavailable(
          "CI/CD health data is unavailable. It is generated by the docs build; " +
          "it will not exist when serving the docs locally."));
      });
  }

  function init() {
    var root = document.getElementById("ci-health-root");
    if (root) load(root);
  }

  // Material's instant navigation swaps pages without reloading scripts.
  if (typeof document$ !== "undefined") {
    document$.subscribe(init);
  } else if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
