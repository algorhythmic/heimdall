import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "david.heimdall"
  ipcTarget: "david.heimdall"

  readonly property color foreground: bar ? bar.foreground : Color.foreground
  readonly property color urgent: bar ? bar.urgent : Color.urgent
  readonly property color accent: Color.accent
  readonly property color dim: Qt.darker(foreground, 1.55)
  readonly property color track: Qt.rgba(foreground.r, foreground.g, foreground.b, 0.16)
  readonly property string fontFamily: bar ? bar.fontFamily : Style.font.family

  // ---- daemon data ------------------------------------------------------

  property bool connected: false
  property var snapshot: ({})
  property var agentRows: []
  property double nowMs: Date.now()
  property int lastNeeds: -1
  property string notifyText: ""

  readonly property var needs: snapshot.needs || []
  readonly property var statusCounts: snapshot.agents || ({})
  readonly property int agentsTotal: Number(snapshot.agents_total || 0)
  readonly property int agentsUnbound: Number(snapshot.agents_unbound || 0)
  readonly property var workstreams: snapshot.workstreams || []
  readonly property int snapshotAgeSec: Number(snapshot.latest_snapshot_age_seconds || 0)

  readonly property int refreshIntervalSec: Number(setting("refreshIntervalSec", 5))
  readonly property bool notifyOnNeeds: setting("notifyOnNeeds", true) !== false
  readonly property int snapshotWarnMin: Math.max(1, Number(setting("snapshotWarnMin", 10)))
  readonly property int unsavedWarnDays: Math.max(1, Number(setting("unsavedWarnDays", 5)))
  readonly property bool snapshotStale: connected && snapshotAgeSec > snapshotWarnMin * 60

  // An unsaved need stops counting toward attention while its checkpoint is
  // younger than unsavedWarnDays; never-saved workstreams always count.
  function savedSecFor(id) {
    for (var i = 0; i < workstreams.length; i++)
      if (workstreams[i].id === id) return Number(workstreams[i].saved_age_seconds)
    return -1
  }
  function needsAttention(n) {
    if (n.kind !== "unsaved") return true
    var sec = savedSecFor(n.target)
    return sec < 0 || sec >= unsavedWarnDays * 86400
  }
  readonly property var visibleNeeds: {
    var out = []
    for (var i = 0; i < needs.length; i++)
      if (needsAttention(needs[i])) out.push(needs[i])
    return out
  }
  readonly property int alarmCount: visibleNeeds.length + (snapshotStale ? 1 : 0)
  readonly property bool alarming: connected && alarmCount > 0

  // ---- actions ----------------------------------------------------------

  function refresh() {
    if (!statusProc.running) statusProc.running = true
  }

  function openTui() {
    if (root.bar) root.bar.run("omarchy-launch-floating-terminal-with-presentation heimdall")
    root.close()
  }

  function persistSettings(values) {
    var entry = { "id": root.moduleName }
    for (var k in root.settings) if (k !== "id") entry[k] = root.settings[k]
    for (var key in values) entry[key] = values[key]
    root.settings = entry
    if (root.bar && root.bar.shell && typeof root.bar.shell.updateEntryInline === "function")
      root.bar.shell.updateEntryInline(root.moduleName, entry)
  }

  // ---- display helpers --------------------------------------------------

  function needGlyph(kind) {
    return (kind === "uncertain" || kind === "agent") ? "?" : "●"
  }
  function needColor(kind) {
    if (kind === "completion" || kind === "decision" || kind === "artifact" || kind === "agent") return urgent
    if (kind === "link" || kind === "uncertain") return accent
    return dim
  }

  function ageText(sec) {
    if (sec < 0) return "never"
    var s = Math.floor(sec)
    if (s < 60) return s + "s"
    var m = Math.floor(s / 60)
    if (m < 60) return m + "m"
    var h = Math.floor(m / 60)
    if (h < 24) return h + "h"
    var d = Math.floor(h / 24)
    if (d < 30) return d + "d"
    var dt = new Date(nowMs - sec * 1000)
    return ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"][dt.getMonth()] + " " + dt.getDate()
  }

  function rebuildAgentRows() {
    var spec = [["working", "●", accent], ["blocked", "?", urgent], ["idle", "○", dim], ["done", "✓", foreground]]
    var rows = []
    var groups = snapshot.agent_groups || []
    for (var i = 0; i < groups.length; i++) {
      var g = groups[i], dots = []
      for (var s = 0; s < spec.length; s++) {
        var n = Number(g[spec[s][0]] || 0)
        for (var k = 0; k < n; k++) dots.push({ "t": spec[s][1], "c": spec[s][2] })
      }
      rows.push({ "target": String(g.target || ""), "dots": dots })
    }
    agentRows = rows
  }

  function afterParse() {
    rebuildAgentRows()
    var n = visibleNeeds.length
    if (notifyOnNeeds && lastNeeds >= 0 && n > lastNeeds) {
      notifyText = n + (n === 1 ? " item needs" : " items need") + " you"
      notifyProc.running = true
    }
    lastNeeds = n
  }

  // ---- widget ------------------------------------------------------------

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  Process {
    id: statusProc
    command: ["heimdall", "status", "--json"]
    stdout: StdioCollector {
      onStreamFinished: {
        try {
          root.snapshot = JSON.parse(text)
          root.connected = true
          root.nowMs = Date.now()
          root.afterParse()
        } catch (e) {
          root.connected = false
        }
      }
    }
    onExited: function(code) { if (code !== 0) root.connected = false }
  }

  Process {
    id: notifyProc
    command: ["notify-send", "Heimdall", root.notifyText]
  }

  Timer {
    interval: Math.max(2, root.refreshIntervalSec) * 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: { root.nowMs = Date.now(); root.refresh() }
  }

  // Ages keep ticking while the panel sits open between daemon polls.
  Timer {
    interval: 30000
    running: root.opened
    repeat: true
    onTriggered: root.nowMs = Date.now()
  }

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: "󰈈"
    active: root.alarming
    opacity: root.connected ? 1.0 : 0.45
    tooltipText: root.connected
      ? root.alarmCount + " open · " + root.agentsTotal + " agents · snapshot " + root.ageText(root.snapshotAgeSec)
      : "Heimdall daemon unavailable"
    onPressed: function(b) {
      if (b === Qt.RightButton) root.refresh()
      else if (b === Qt.MiddleButton) root.openTui()
      else root.toggle()
    }
  }

  // ---- panel --------------------------------------------------------------

  KeyboardPanel {
    id: panel
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(360))
    contentHeight: panel.fittedContentHeight(column.implicitHeight, Style.space(620))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      blocked: staleField.field.activeFocus || unsavedField.field.activeFocus
      onMoveRequested: function(dx, dy) {
        if (dy !== 0)
          flick.contentY = Math.max(0, Math.min(flick.contentY + dy * Style.space(56),
                                                Math.max(0, flick.contentHeight - flick.height)))
      }
      onActivateRequested: root.openTui()
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }
      onTextKey: function(t) { if (t === "r" || t === "R") root.refresh() }

      Flickable {
        id: flick
        anchors.fill: parent
        contentWidth: width
        contentHeight: column.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.VerticalFlick
        interactive: contentHeight > height
        ScrollBar.vertical: ScrollBar { policy: ScrollBar.AsNeeded }

        Column {
          id: column
          width: flick.width
          spacing: Style.space(10)

          PanelHero {
            width: parent.width
            title: "heimdall"
            meta: root.connected
              ? "daemon ok · snapshot " + root.ageText(root.snapshotAgeSec) + (root.snapshotStale ? " · stale" : "")
              : "daemon unavailable"
            detail: root.visibleNeeds.length > 0 ? String(root.visibleNeeds.length) : ""
            foreground: root.foreground
            fontFamily: root.fontFamily
            iconComponent: Component {
              Text {
                textFormat: Text.PlainText
                text: "󰈈"
                color: root.foreground
                font.family: root.fontFamily
                font.pixelSize: Style.font.display
              }
            }
          }

          Text {
            visible: !root.connected
            width: parent.width
            text: "The daemon is not answering.\nStart it with `heimdall serve`; this panel reconnects on its own."
            color: root.dim
            font.family: root.fontFamily
            font.pixelSize: Style.font.body
            wrapMode: Text.WordWrap
          }

          // ---------------- needs ----------------

          PanelSectionHeader {
            visible: root.connected
            text: "NEEDS YOU · " + root.visibleNeeds.length
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          Column {
            id: needsColumn
            visible: root.connected
            width: parent.width
            spacing: Style.space(6)

            Text {
              visible: root.visibleNeeds.length === 0
              text: "nothing needs you"
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }

            Repeater {
              model: root.visibleNeeds.slice(0, 10)
              Row {
                required property var modelData
                width: needsColumn.width
                spacing: Style.space(8)

                Text {
                  id: needBullet
                  textFormat: Text.PlainText
                  text: root.needGlyph(modelData.kind)
                  color: root.needColor(modelData.kind)
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                }
                Text {
                  textFormat: Text.PlainText
                  width: needsColumn.width - parent.spacing - needBullet.implicitWidth
                  text: modelData.target ? modelData.target + " · " + modelData.text : modelData.text
                  color: root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  elide: Text.ElideRight
                }
              }
            }

            Text {
              visible: root.visibleNeeds.length > 10
              text: "+ " + (root.visibleNeeds.length - 10) + " more"
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }

          // ---------------- agents ----------------

          PanelSectionHeader {
            visible: root.connected && root.agentsTotal > 0
            text: "AGENTS · " + root.agentsTotal + " IN HERDR"
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          Column {
            visible: root.connected && root.agentsTotal > 0
            width: parent.width
            spacing: Style.space(6)

            Row {
              spacing: Style.space(10)
              Text { visible: (root.statusCounts.working || 0) > 0; text: (root.statusCounts.working || 0) + " working"; color: root.accent; font.family: root.fontFamily; font.pixelSize: Style.font.body }
              Text { visible: (root.statusCounts.blocked || 0) > 0; text: (root.statusCounts.blocked || 0) + " blocked"; color: root.urgent; font.family: root.fontFamily; font.pixelSize: Style.font.body }
              Text { visible: (root.statusCounts.idle || 0) > 0; text: (root.statusCounts.idle || 0) + " idle"; color: root.dim; font.family: root.fontFamily; font.pixelSize: Style.font.body }
              Text { visible: (root.statusCounts.done || 0) > 0; text: (root.statusCounts.done || 0) + " done"; color: root.foreground; font.family: root.fontFamily; font.pixelSize: Style.font.body }
            }

            Flow {
              width: parent.width
              spacing: Style.space(12)

              Repeater {
                model: root.agentRows
                Row {
                  required property var modelData
                  spacing: 3
                  Text {
                    textFormat: Text.PlainText
                    text: modelData.target
                    color: root.foreground
                    font.family: root.fontFamily
                    font.pixelSize: Style.font.body
                  }
                  Repeater {
                    model: modelData.dots
                    Text {
                      textFormat: Text.PlainText
                      text: modelData.t
                      color: modelData.c
                      font.family: root.fontFamily
                      font.pixelSize: Style.font.body
                    }
                  }
                }
              }
            }

            Text {
              visible: root.agentsUnbound > 0
              text: root.agentsUnbound + " unbound · bind from the window"
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.caption
            }
          }

          // ---------------- workstreams ----------------

          PanelSectionHeader {
            visible: root.connected && root.workstreams.length > 0
            text: "WORKSTREAMS"
            foreground: root.foreground
            fontFamily: root.fontFamily
          }

          Column {
            id: wsColumn
            visible: root.connected && root.workstreams.length > 0
            width: parent.width
            spacing: Style.space(8)

            Repeater {
              model: root.workstreams.slice(0, 8)
              Item {
                id: wsRow
                required property var modelData
                readonly property int filled: Number(modelData.steps_total) > 0
                  ? Math.round(Number(modelData.steps_done) / Number(modelData.steps_total) * 8) : 0
                width: wsColumn.width
                height: wsName.implicitHeight

                Text {
                  id: wsName
                  anchors.left: parent.left
                  anchors.verticalCenter: parent.verticalCenter
                  width: wsRow.width - wsBar.width - wsAge.implicitWidth - Style.space(16)
                  textFormat: Text.PlainText
                  text: modelData.id
                  color: root.foreground
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.body
                  elide: Text.ElideRight
                }

                Row {
                  id: wsBar
                  anchors.right: wsAge.left
                  anchors.rightMargin: Style.space(10)
                  anchors.verticalCenter: parent.verticalCenter
                  spacing: 2
                  Repeater {
                    model: 8
                    Rectangle {
                      width: 7
                      height: 6
                      color: index < wsRow.filled ? root.accent : root.track
                    }
                  }
                }

                Text {
                  id: wsAge
                  anchors.right: parent.right
                  anchors.verticalCenter: parent.verticalCenter
                  textFormat: Text.PlainText
                  text: root.ageText(Number(modelData.saved_age_seconds))
                  color: root.dim
                  font.family: root.fontFamily
                  font.pixelSize: Style.font.caption
                }
              }
            }
          }

          // ---------------- settings + actions ----------------

          PanelSeparator {
            visible: root.connected
            foreground: root.foreground
          }

          Toggle {
            visible: root.connected
            width: parent.width
            label: "Notify when something needs you"
            checked: root.notifyOnNeeds
            foreground: root.foreground
            accent: root.accent
            fontFamily: root.fontFamily
            onClicked: root.persistSettings({ "notifyOnNeeds": !root.notifyOnNeeds })
          }

          Item {
            visible: root.connected
            width: parent.width
            height: staleField.implicitHeight
            Text {
              anchors.left: parent.left
              anchors.verticalCenter: parent.verticalCenter
              text: "Warn when snapshot older than (min)"
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
            NumberField {
              id: staleField
              anchors.right: parent.right
              value: root.snapshotWarnMin
              from: 1
              to: 1440
              fieldWidth: Style.space(80)
              fontFamily: root.fontFamily
              foreground: root.foreground
              accent: root.accent
              onModified: function(v) { root.persistSettings({ "snapshotWarnMin": v }) }
            }
          }

          Item {
            visible: root.connected
            width: parent.width
            height: unsavedField.implicitHeight
            Text {
              anchors.left: parent.left
              anchors.verticalCenter: parent.verticalCenter
              text: "Unsaved warning after (days)"
              color: root.dim
              font.family: root.fontFamily
              font.pixelSize: Style.font.body
            }
            NumberField {
              id: unsavedField
              anchors.right: parent.right
              value: root.unsavedWarnDays
              from: 1
              to: 30
              fieldWidth: Style.space(80)
              fontFamily: root.fontFamily
              foreground: root.foreground
              accent: root.accent
              onModified: function(v) { root.persistSettings({ "unsavedWarnDays": v }) }
            }
          }

          Row {
            visible: root.connected
            width: parent.width
            spacing: Style.spacing.md

            Button {
              id: openButton
              text: "Open window"
              bordered: true
              fontFamily: root.fontFamily
              foreground: root.foreground
              accent: root.accent
              onClicked: root.openTui()
            }
            Item { width: parent.width - openButton.implicitWidth - refreshButton.implicitWidth - parent.spacing * 2; height: 1 }
            Button {
              id: refreshButton
              text: "Refresh"
              bordered: true
              fontFamily: root.fontFamily
              foreground: root.foreground
              accent: root.accent
              onClicked: root.refresh()
            }
          }
        }
      }
    }
  }
}
