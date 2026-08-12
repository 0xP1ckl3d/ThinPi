import QtQuick
import QtQuick.Controls

Button {
    id: control
    property string variant: "secondary"
    property color accentColor: variant === "primary" ? "#06131b" : variant === "danger" ? "#ffb5bf" : "#e8f1fb"
    implicitHeight: 44
    implicitWidth: Math.max(96, contentItem.implicitWidth + 34)
    leftPadding: 17
    rightPadding: 17
    contentItem: Label {
        text: control.text
        color: control.enabled ? control.accentColor : "#66788d"
        font.pixelSize: 14
        font.weight: Font.DemiBold
        horizontalAlignment: Text.AlignHCenter
        verticalAlignment: Text.AlignVCenter
        elide: Text.ElideRight
    }
    background: Rectangle {
        radius: 10
        border.width: 1
        border.color: control.variant === "primary" ? "#65dfc2" : control.variant === "danger" ? "#7a3547" : "#31506f"
        color: {
            if (!control.enabled) return "#111d2b"
            if (control.variant === "primary") return control.down ? "#4ec3a9" : control.hovered ? "#71e8ce" : "#5ed9bd"
            if (control.variant === "danger") return control.down ? "#351923" : control.hovered ? "#321923" : "#281720"
            return control.down ? "#16283d" : control.hovered ? "#203b57" : "#172b42"
        }
        Behavior on color { ColorAnimation { duration: 120 } }
    }
}
