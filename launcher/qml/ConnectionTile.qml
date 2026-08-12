import QtQuick
import QtQuick.Controls

Rectangle {
    id: tile; property string title; property string subtitle; property string badge; signal activated
    radius: 14; color: mouse.containsMouse ? "#1b3551" : "#12243a"; border.color: activeFocus ? "#62dbc5" : "#27425f"; border.width: activeFocus ? 3 : 1
    Accessible.name: title+" "+badge; Accessible.role: Accessible.Button; Accessible.onPressAction: activated(); focus: true
    Column { anchors.fill: parent; anchors.margins: 22; spacing: 10
        Rectangle { width: protocol.width+16; height: 27; radius: 13; color: "#245d61"; Label { id: protocol; anchors.centerIn: parent; text: tile.badge; color: "#9ff5e6"; font.bold: true } }
        Label { text: tile.title; color: "white"; font.pixelSize: 24; font.weight: Font.DemiBold; elide: Text.ElideRight; width: parent.width }
        Label { text: tile.subtitle; color: "#aebed2"; font.pixelSize: 15; elide: Text.ElideRight; width: parent.width }
    }
    MouseArea { id: mouse; anchors.fill: parent; hoverEnabled: true; onClicked: tile.activated() }
    Keys.onReturnPressed: activated(); Keys.onEnterPressed: activated(); Keys.onSpacePressed: activated()
}
