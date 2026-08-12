import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Dialog {
    id: dialog
    property string message
    property bool confirmSSHHostKey: false
    modal: true
    anchors.centerIn: Overlay.overlay
    width: Math.min(560, Overlay.overlay.width - 48)
    closePolicy: Popup.NoAutoClose
    padding: 24
    background: Rectangle {
        color: "#13233a"
        radius: 14
        border.color: "#294564"
        border.width: 1
    }
    contentItem: ColumnLayout {
        spacing: 14
        Label {
            text: "Unable to complete request"
            color: "#f4f8ff"
            font.pixelSize: 24
            font.weight: Font.DemiBold
        }
        Label {
            Layout.fillWidth: true
            text: dialog.message
            color: "#c8d6e8"
            font.pixelSize: 16
            wrapMode: Text.Wrap
        }
        RowLayout {
            Layout.alignment: Qt.AlignRight
            ThinButton {
                text: dialog.confirmSSHHostKey ? "Cancel" : "Return to ThinPi"
                onClicked: { if (dialog.confirmSSHHostKey) backend.resolveSSHHostKey(false); dialog.close() }
            }
            ThinButton {
                visible: dialog.confirmSSHHostKey
                text: "Trust new key"
                variant: "primary"
                onClicked: { backend.resolveSSHHostKey(true); dialog.close() }
            }
        }
    }
}
