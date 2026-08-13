import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Window

Window {
    id: controls
    property bool revealed: false
    property bool pinned: false
    width: 410
    height: revealed || pinned ? 52 : 3
    x: Math.round((Screen.width-width)/2)
    y: 0
    color: "transparent"
    flags: Qt.Tool | Qt.FramelessWindowHint | Qt.WindowStaysOnTopHint | Qt.WindowDoesNotAcceptFocus
    onVisibleChanged: if (!visible) { revealed = false; pinned = false }

    Rectangle {
        visible: controls.revealed || controls.pinned
        width: parent.width
        height: 52
        anchors.top: parent.top
        anchors.horizontalCenter: parent.horizontalCenter
        color: "#091421ee"
        radius: 0
        border.color: "#34506b"
        RowLayout {
            anchors.fill: parent
            anchors.margins: 7
            spacing: 8
            Rectangle {
                id: dragHandle
                color: "transparent"
                Layout.fillWidth: true
                Layout.fillHeight: true
                Label { anchors.centerIn: parent; text: "⋮⋮  " + backend.sessionMessage; color: "#dbe7f5"; elide: Text.ElideRight; width: parent.width; horizontalAlignment: Text.AlignHCenter; font.pixelSize: 12 }
                DragHandler {
                    target: null
                    property real startX: 0
                    onActiveChanged: if (active) startX = controls.x
                    onTranslationChanged: controls.x = Math.max(0, Math.min(Screen.width-controls.width, startX+translation.x))
                }
            }
            ThinButton { text: controls.pinned ? "Unpin" : "Pin"; onClicked: { controls.pinned = !controls.pinned; controls.revealed = true } }
            ThinButton { text: "Minimise"; onClicked: backend.minimizeSession() }
            ThinButton { text: "Close"; variant: "danger"; onClicked: backend.endSession() }
        }
    }
    HoverHandler {
        onHoveredChanged: {
            if (hovered) { hideTimer.stop(); controls.revealed = true }
            else if (!controls.pinned) hideTimer.restart()
        }
    }
    Timer { id: hideTimer; interval: 300; onTriggered: if (!controls.pinned) controls.revealed = false }
}
