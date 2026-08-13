import QtQuick
import QtQuick.Controls

Rectangle {
    id: tile; property string title; property string subtitle; property string badge; property string sessionState: ""; signal activated
    Theme { id: theme; paletteName: backend.clientTheme }
    radius: 14; color: mouse.containsMouse ? theme.surfaceHover : theme.surface; border.color: activeFocus ? theme.accent : theme.border; border.width: activeFocus ? 3 : 1
    Accessible.name: title+" "+badge; Accessible.role: Accessible.Button; Accessible.onPressAction: activated(); activeFocusOnTab: true
    Column { anchors.fill: parent; anchors.margins: 22; spacing: 10
        Rectangle { width: protocol.width+16; height: 27; radius: 13; color: theme.backgroundAlt; Label { id: protocol; anchors.centerIn: parent; text: tile.badge; color: theme.accent; font.bold: true } }
        Label { text: tile.title; color: theme.text; font.pixelSize: 24; font.weight: Font.DemiBold; elide: Text.ElideRight; width: parent.width }
        Label { text: tile.subtitle; color: theme.muted; font.pixelSize: 15; elide: Text.ElideRight; width: parent.width }
        Rectangle { visible: tile.sessionState.length > 0; width: sessionText.width+18; height: 25; radius: 12; color: tile.sessionState === "minimized" ? theme.accent : theme.backgroundAlt; Label { id: sessionText; anchors.centerIn: parent; text: tile.sessionState === "minimized" ? "MINIMIZED · SELECT TO REOPEN" : tile.sessionState === "active" ? "OPEN" : tile.sessionState.replace(/_/g, " ").toUpperCase(); color: tile.sessionState === "minimized" ? theme.accentText : theme.accent; font.pixelSize: 10; font.bold: true } }
    }
    MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; onClicked: tile.activated() }
    Keys.onReturnPressed: activated(); Keys.onEnterPressed: activated(); Keys.onSpacePressed: activated()
}
