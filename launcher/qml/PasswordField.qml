import QtQuick
import QtQuick.Controls

Item {
    id: root
    property alias text: field.text
    property alias placeholderText: field.placeholderText
    signal accepted
    function focusInput() { field.forceActiveFocus() }
    implicitHeight: 50
    implicitWidth: 320

    ThinTextField {
        id: field
        anchors.fill: parent
        enabled: root.enabled
        rightPadding: 54
        echoMode: reveal.checked ? TextInput.Normal : TextInput.Password
        onAccepted: root.accepted()
    }

    ThinButton {
        id: reveal
        anchors.right: parent.right
        anchors.rightMargin: 5
        anchors.verticalCenter: parent.verticalCenter
        width: 42
        height: 38
        implicitWidth: 42
        implicitHeight: 38
        checkable: true
        text: "👁"
        variant: "secondary"
        Accessible.name: checked ? "Hide password" : "Show password"
        background: Rectangle { color: "transparent"; radius: 8 }
        contentItem: Label { text: reveal.text; color: reveal.checked ? "#5ed9bd" : "#8ca2ba"; font.pixelSize: 18; horizontalAlignment: Text.AlignHCenter; verticalAlignment: Text.AlignVCenter }
    }
}
