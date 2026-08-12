import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Dialog {
    id: dialog
    property string message
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
        ThinButton {
            Layout.alignment: Qt.AlignRight
            text: "Return to ThinPi"
            variant: "primary"
            onClicked: dialog.close()
        }
    }
}
