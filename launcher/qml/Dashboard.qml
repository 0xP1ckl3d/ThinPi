import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    id: dashboardRoot
    color: "#07111e"
    ColumnLayout { anchors.fill: parent; anchors.margins: 42; spacing: 22
        RowLayout { Layout.fillWidth: true
            ColumnLayout { Label { text: "Welcome, "+backend.displayName; color: "white"; font.pixelSize: 31; font.weight: Font.DemiBold } Label { text: "Choose a remote system"; color: "#9fb0c8"; font.pixelSize: 17 } }
            Item { Layout.fillWidth: true } Button { visible: backend.isAdmin; text: "Administration"; onClicked: backend.openAdministration() } Button { visible: backend.isAdmin && !backend.devMode; text: "Local maintenance"; onClicked: maintenanceConfirm.open() } Button { text: "Refresh"; onClicked: backend.refresh() } Button { text: "Log out"; onClicked: backend.logout() }
        }
        Rectangle { visible: backend.restrictionMessage.length > 0; Layout.fillWidth: true; implicitHeight: restrictionRow.implicitHeight+28; radius: 12; color: "#3a2415"; border.color: "#75502a"
            RowLayout { id: restrictionRow; anchors.fill: parent; anchors.margins: 14; spacing: 12
                Rectangle { width: 32; height: 32; radius: 16; color: "#61411f"; Label { anchors.centerIn: parent; text: "!"; color: "#ffd083"; font.bold: true } }
                ColumnLayout { Layout.fillWidth: true; spacing: 2; Label { text: "Access is currently paused"; color: "#ffe0a8"; font.bold: true } Label { text: backend.restrictionMessage; color: "#d8b98a" } }
            }
        }
        GridView { id: grid; Layout.fillWidth: true; Layout.fillHeight: true; cellWidth: 310; cellHeight: 190; clip: true; model: backend.connections
            delegate: ConnectionTile { width: 288; height: 168; title: connectionName; subtitle: description; badge: protocol; onActivated: backend.launch(index) }
            Label { anchors.centerIn: parent; visible: grid.count===0&&!backend.busy; text: "No remote systems have been assigned to you."; color: "#9fb0c8"; font.pixelSize: 18 }
        }
        BusyIndicator { running: backend.busy; visible: running; Layout.alignment: Qt.AlignHCenter }
    }
    Dialog {
        id: maintenanceConfirm
        anchors.centerIn: parent
        modal: true
        width: 560
        padding: 24
        background: Rectangle { color: "#13233a"; radius: 14; border.color: "#294564" }
        contentItem: ColumnLayout {
            spacing: 16
            Label { text: "Open local maintenance console?"; color: "#f4f8ff"; font.pixelSize: 24; font.weight: Font.DemiBold }
            Label {
                Layout.fillWidth: true
                text: "The ThinPi app will sign out and switch to this device's administrator console.\nType exit when maintenance is complete to return to the locked launcher."
                color: "#c8d6e8"
                wrapMode: Text.Wrap
            }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                Button { text: "Cancel"; onClicked: maintenanceConfirm.close() }
                Button { text: "Open console"; onClicked: { maintenanceConfirm.close(); backend.openMaintenance() } }
            }
        }
    }
}
