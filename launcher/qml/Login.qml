import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    color: "#07111e"
    ColumnLayout { anchors.centerIn: parent; width: Math.min(parent.width-80,430); spacing: 18
        Image { source: "qrc:/qt/qml/ThinPi/assets/thinpi.svg"; sourceSize.width: 88; sourceSize.height: 88; Layout.alignment: Qt.AlignHCenter }
        Label { text: "ThinPi"; color: "white"; font.pixelSize: 42; font.weight: Font.DemiBold; Layout.alignment: Qt.AlignHCenter }
        Label { text: "Sign in to your remote systems"; color: "#9fb0c8"; font.pixelSize: 17; Layout.alignment: Qt.AlignHCenter }
        TextField { id: username; placeholderText: "Username"; Layout.fillWidth: true; font.pixelSize: 19; enabled: !backend.busy; focus: true; onAccepted: password.forceActiveFocus() }
        TextField { id: password; placeholderText: "Password"; echoMode: TextInput.Password; Layout.fillWidth: true; font.pixelSize: 19; enabled: !backend.busy; onAccepted: backend.login(username.text,password.text) }
        Button { text: backend.busy ? "Signing in…" : "Sign in"; Layout.fillWidth: true; enabled: !backend.busy; highlighted: true; onClicked: backend.login(username.text,password.text) }
        BusyIndicator { running: backend.busy; visible: running; Layout.alignment: Qt.AlignHCenter }
    }
}
