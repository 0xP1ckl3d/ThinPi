import QtQuick
import QtQuick.Controls

TextField {
    id: control
    Theme { id: theme; paletteName: backend.clientTheme }
    implicitHeight: 50
    leftPadding: 15
    rightPadding: 15
    color: theme.text
    placeholderTextColor: theme.muted
    selectionColor: theme.accent
    selectedTextColor: theme.accentText
    font.pixelSize: 16
    background: Rectangle {
        radius: 11
        color: theme.backgroundAlt
        border.width: control.activeFocus ? 2 : 1
        border.color: control.activeFocus ? theme.accent : theme.border
    }
}
