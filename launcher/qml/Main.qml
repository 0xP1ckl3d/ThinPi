import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import ThinPi

ApplicationWindow {
    id: root; visible: true; visibility: backend.devMode ? Window.Windowed : Window.FullScreen
    width: 1100; height: 700; title: "ThinPi"; color: appTheme.background
    font.family: "sans-serif"
    Theme { id: appTheme; paletteName: backend.clientTheme }
    Loader { anchors.fill: parent; sourceComponent: backend.view === "login" ? login : backend.view === "dashboard" ? dashboard : backend.view === "session" ? session : offline; onLoaded: { if (item) item.forceActiveFocus() } }
    Component { id: login; Login {} }
    Component { id: dashboard; Dashboard {} }
    Component { id: session; SessionOverlay {} }
    Component { id: offline; Rectangle { color: root.color; ColumnLayout { anchors.centerIn: parent; spacing: 20; Label { text: "Controller unavailable"; color: "white"; font.pixelSize: 34 } Label { text: backend.errorMessage; color: "#aebed2"; wrapMode: Text.Wrap; Layout.maximumWidth: 600 } ThinButton { text: "Try again"; variant: "primary"; Layout.alignment: Qt.AlignHCenter; onClicked: backend.retry() } } } }
    ErrorDialog { visible: backend.errorMessage.length > 0 && backend.view !== "offline"; message: backend.errorMessage; confirmSSHHostKey: backend.sshHostKeyConfirmation; onClosed: { if (!backend.sshHostKeyConfirmation) backend.dismissError() } }
    Connections { target: backend; function onSessionActiveChanged() { if (backend.devMode) return; if (backend.sessionActive) root.hide(); else { root.showFullScreen(); root.raise(); root.requestActivate(); } } }
}
