import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Window

Window {
    id: controls
    property bool revealed: false
    property bool pinned: false
    property bool dragging: false
    property bool interacting: false
    property bool suppressInteraction: false
    property real dragOffsetX: 0
    readonly property bool deployed: revealed || pinned || dragging
    width: 340
    height: 42
    x: Math.round((Screen.width - width) / 2)
    y: deployed ? 0 : -height
    color: "transparent"
    // The strip only needs pointer input. Keeping it out of the X11 keyboard
    // focus chain prevents hovering or clicking it from taking keyboard input
    // away from the active remote client.
    flags: Qt.Tool | Qt.FramelessWindowHint | Qt.WindowStaysOnTopHint | Qt.X11BypassWindowManagerHint | Qt.WindowDoesNotAcceptFocus
    function activateInteraction() {
        if (interacting || suppressInteraction)
            return
        backend.beginToolbarInteraction()
        interacting = true
        controls.raise()
    }
    function deactivateInteraction() {
        if (!interacting)
            return
        interacting = false
        backend.endToolbarInteraction()
    }
    function prepareAction() {
        suppressInteraction = true
        interacting = false
    }
    onVisibleChanged: {
        if (!visible)
            deactivateInteraction()
        revealed = false
        pinned = false
        dragging = false
        suppressInteraction = false
        if (visible)
            edgePoll.restart()
    }
    onDeployedChanged: {
        if (deployed)
            controls.raise()
        else
            deactivateInteraction()
    }
    Behavior on y { NumberAnimation { duration: 130; easing.type: Easing.OutCubic } }

    Theme { id: theme; paletteName: backend.clientTheme }

    Rectangle {
        id: bar
        anchors.fill: parent
        color: theme.surface
        radius: 9
        border.width: 1
        border.color: theme.border

        RowLayout {
            anchors.fill: parent
            anchors.margins: 5
            spacing: 5

            Item {
                id: dragHandle
                Layout.fillWidth: true
                Layout.fillHeight: true

                RowLayout {
                    anchors.fill: parent
                    spacing: 6
                    Label { text: "⠿"; color: theme.muted; font.pixelSize: 15 }
                    Label {
                        Layout.fillWidth: true
                        text: backend.sessionMessage
                        color: theme.text
                        elide: Text.ElideRight
                        font.pixelSize: 12
                        font.weight: Font.DemiBold
                    }
                }

                DragHandler {
                    target: null
                    onActiveChanged: {
                        controls.dragging = active
                        if (active) {
                            hideTimer.stop()
                            controls.revealed = true
                            controls.activateInteraction()
                            controls.dragOffsetX = backend.pointerX() - controls.x
                        } else if (!controls.pinned) {
                            hideTimer.restart()
                        }
                    }
                }
            }

            Button {
                id: pinButton
                text: controls.pinned ? "Unpin" : "Pin"
                implicitWidth: 51
                implicitHeight: 30
                focusPolicy: Qt.NoFocus
                onClicked: {
                    controls.pinned = !controls.pinned
                    controls.revealed = true
                    hideTimer.stop()
                }
                contentItem: Label { text: pinButton.text; color: theme.text; horizontalAlignment: Text.AlignHCenter; verticalAlignment: Text.AlignVCenter; font.pixelSize: 11; font.weight: Font.DemiBold }
                background: Rectangle { radius: 6; color: pinButton.hovered ? theme.surfaceHover : theme.backgroundAlt; border.color: theme.border }
            }
            Button {
                id: minimizeButton
                text: "—"
                Accessible.name: "Minimise connection"
                implicitWidth: 38
                implicitHeight: 30
                focusPolicy: Qt.NoFocus
                onClicked: {
                    controls.prepareAction()
                    backend.minimizeSession()
                }
                contentItem: Label { text: minimizeButton.text; color: theme.text; horizontalAlignment: Text.AlignHCenter; verticalAlignment: Text.AlignVCenter; font.pixelSize: 18; font.bold: true }
                background: Rectangle { radius: 6; color: minimizeButton.hovered ? theme.surfaceHover : theme.backgroundAlt; border.color: theme.border }
            }
            Button {
                id: closeButton
                text: "×"
                Accessible.name: "Close connection"
                implicitWidth: 38
                implicitHeight: 30
                focusPolicy: Qt.NoFocus
                onClicked: {
                    controls.prepareAction()
                    backend.endSession()
                }
                contentItem: Label { text: closeButton.text; color: "#ffb5bf"; horizontalAlignment: Text.AlignHCenter; verticalAlignment: Text.AlignVCenter; font.pixelSize: 19; font.bold: true }
                background: Rectangle { radius: 6; color: closeButton.hovered ? "#3b1c27" : "#281720"; border.color: "#7a3547" }
            }
        }

        HoverHandler {
            onHoveredChanged: {
                if (hovered) {
                    hideTimer.stop()
                    controls.revealed = true
                    controls.activateInteraction()
                } else if (!controls.pinned && !controls.dragging) {
                    hideTimer.restart()
                }
            }
        }
    }

    Timer {
        id: edgePoll
        interval: 60
        repeat: true
        running: controls.visible && !controls.deployed
        onTriggered: {
            if (backend.pointerAtScreenTop()) {
                controls.revealed = true
                controls.raise()
                controls.activateInteraction()
            }
        }
    }
    Timer {
        id: pointerPresence
        interval: 60
        repeat: true
        running: controls.visible && controls.deployed
        onTriggered: {
            if (controls.dragging || controls.suppressInteraction)
                return
            if (backend.pointerOverToolbar(controls.x, controls.width, controls.height)) {
                hideTimer.stop()
                controls.activateInteraction()
            } else {
                controls.deactivateInteraction()
                if (!controls.pinned && !hideTimer.running)
                    hideTimer.restart()
            }
        }
    }
    Timer {
        id: dragTimer
        interval: 16
        repeat: true
        running: controls.visible && controls.dragging
        onTriggered: controls.x = Math.max(0, Math.min(Screen.width - controls.width, backend.pointerX() - controls.dragOffsetX))
    }
    Timer {
        id: raiseTimer
        interval: 250
        repeat: true
        running: controls.visible && controls.deployed
        onTriggered: controls.raise()
    }
    Timer {
        id: hideTimer
        interval: 650
        onTriggered: if (!controls.pinned && !controls.dragging) controls.revealed = false
    }
}
