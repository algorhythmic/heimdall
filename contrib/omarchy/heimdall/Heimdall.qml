import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

BarWidget {
  id: root
  moduleName: "david.heimdall"

  property string statusText: ""
  property string tip: "Heimdall"

  function refresh() {
    if (!statusProc.running) statusProc.running = true
  }

  function openTui() {
    if (root.bar) root.bar.run("omarchy-launch-floating-terminal-with-presentation heimdall")
  }

  visible: statusText !== ""
  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  Process {
    id: statusProc
    command: ["heimdall", "status", "--json"]
    stdout: StdioCollector {
      onStreamFinished: {
        try {
          var d = JSON.parse(text)
          var a = d.agents || {}
          var dots = ""
          if (d.agents_total > 0) {
            dots = "  ●" + (a.working || 0) + " ?" + (a.blocked || 0) + " ○" + (a.idle || 0)
            if ((a.done || 0) > 0) dots += " ✓" + a.done
          }
          root.statusText = "⚠ " + (d.needs_count || 0) + dots
          var lines = []
          var needs = d.needs || []
          for (var i = 0; i < needs.length && i < 12; i++) {
            var n = needs[i]
            lines.push(n.kind + "  " + (n.target || "·") + " — " + n.text)
          }
          root.tip = (lines.length ? lines.join("\n") : "nothing needs you") +
                     "\nsnapshot " + d.latest_snapshot_age + " ago"
        } catch (e) {
          root.statusText = ""
        }
      }
    }
  }

  Timer {
    interval: (root.settings.refreshIntervalSec || 5) * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.statusText
    slotSize: Style.bar.statusSlot
    fontSize: Style.font.caption
    tooltipText: root.tip
    onPressed: function(b) {
      if (b === Qt.RightButton) root.refresh()
      else root.openTui()
    }
  }
}
