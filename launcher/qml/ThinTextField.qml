import QtQuick
import QtQuick.Controls

TextField {
    id: control
    implicitHeight: 50
    leftPadding: 15
    rightPadding: 15
    color: "#f4f8fd"
    placeholderTextColor: "#72879f"
    selectionColor: "#5ed9bd"
    selectedTextColor: "#06131b"
    font.pixelSize: 16
    background: Rectangle {
        radius: 11
        color: "#0b1725"
        border.width: control.activeFocus ? 2 : 1
        border.color: control.activeFocus ? "#5ed9bd" : "#29425e"
    }
}
