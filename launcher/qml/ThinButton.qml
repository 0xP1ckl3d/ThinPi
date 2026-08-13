import QtQuick
import QtQuick.Controls

Button {
    id: control
    Theme { id: theme; paletteName: backend.clientTheme }
    activeFocusOnTab: true
    property string variant: "secondary"
    property color accentColor: variant === "primary" ? theme.accentText : variant === "danger" ? "#ffb5bf" : theme.text
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
        border.width: control.activeFocus ? 3 : 1
        border.color: control.activeFocus ? theme.accent : control.variant === "primary" ? theme.accent : control.variant === "danger" ? "#7a3547" : theme.border
        color: {
            if (!control.enabled) return theme.backgroundAlt
            if (control.variant === "primary") return control.down ? theme.surfaceHover : control.hovered ? Qt.lighter(theme.accent,1.12) : theme.accent
            if (control.variant === "danger") return control.down ? "#351923" : control.hovered ? "#321923" : "#281720"
            return control.down ? theme.backgroundAlt : control.hovered ? theme.surfaceHover : theme.surface
        }
        Behavior on color { ColorAnimation { duration: 120 } }
    }
}
