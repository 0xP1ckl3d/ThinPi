import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import ThinPi

ApplicationWindow {
    id: root; visible: true; visibility: backend.devMode ? Window.Windowed : Window.FullScreen
    width: 1100; height: 700; title: "ThinPi"; color: "#07111e"
    font.family: "sans-serif"
    Loader { anchors.fill: parent; sourceComponent: backend.view === "login" ? login : backend.view === "dashboard" ? dashboard : backend.view === "session" ? session : offline }
    Component { id: login; Login {} }
    Component { id: dashboard; Dashboard {} }
    Component { id: session; SessionOverlay {} }
    Component { id: offline; Rectangle { color: root.color; ColumnLayout { anchors.centerIn: parent; spacing: 20; Label { text: "Controller unavailable"; color: "white"; font.pixelSize: 34 } Label { text: backend.errorMessage; color: "#aebed2"; wrapMode: Text.Wrap; Layout.maximumWidth: 600 } Button { text: "Try again"; onClicked: backend.retry() } } } }
    ErrorDialog { visible: backend.errorMessage.length > 0 && backend.view !== "offline"; message: backend.errorMessage; onClosed: backend.dismissError() }
}
