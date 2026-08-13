import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    id: dashboardRoot
    Theme { id: theme; paletteName: backend.clientTheme }
    color: theme.background
    focus: true
    Rectangle { anchors.fill: parent; gradient: Gradient { GradientStop { position: 0; color: theme.backgroundAlt } GradientStop { position: 0.5; color: theme.background } GradientStop { position: 1; color: Qt.darker(theme.background,1.08) } } }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 38
        spacing: 22

        RowLayout {
            Layout.fillWidth: true
            spacing: 10
            Rectangle { width: 48; height: 48; radius: 15; color: theme.surface; clip: true; Image { id: dashboardPhoto; anchors.fill: parent; source: backend.profilePhotoUrl; fillMode: Image.PreserveAspectCrop; asynchronous: true } Label { anchors.centerIn: parent; visible: dashboardPhoto.status !== Image.Ready; text: backend.displayName.slice(0,1).toUpperCase(); color: theme.accent; font.pixelSize: 22; font.bold: true } }
            ColumnLayout { spacing: 1; Label { text: "Welcome, " + backend.displayName; color: theme.text; font.pixelSize: 29; font.weight: Font.DemiBold } Label { text: "Choose a remote system"; color: theme.muted; font.pixelSize: 15 } }
            Item { Layout.fillWidth: true }
            ThinButton { visible: backend.isAdmin; text: "Administration"; variant: "primary"; onClicked: backend.openAdministration() }
            ThinButton { visible: backend.isAdmin && !backend.devMode; text: "Local maintenance"; onClicked: maintenanceConfirm.open() }
            ThinButton { text: "Edit profile"; onClicked: { profileUsername.text=backend.username; profileDisplayName.text=backend.displayName; currentPassword.text=""; newPassword.text=""; profileDialog.open(); currentPassword.focusInput() } }
            ThinButton { text: "Refresh"; onClicked: backend.refresh() }
            ThinButton { text: "Log out"; variant: "danger"; onClicked: backend.logout() }
        }

        Rectangle {
            visible: backend.restrictionMessage.length > 0
            Layout.fillWidth: true
            implicitHeight: restrictionRow.implicitHeight + 28
            radius: 12
            color: "#3a2415"
            border.color: "#75502a"
            RowLayout { id: restrictionRow; anchors.fill: parent; anchors.margins: 14; spacing: 12; Rectangle { width: 32; height: 32; radius: 16; color: "#61411f"; Label { anchors.centerIn: parent; text: "!"; color: "#ffd083"; font.bold: true } } ColumnLayout { Layout.fillWidth: true; spacing: 2; Label { text: "Access is currently paused"; color: "#ffe0a8"; font.bold: true } Label { text: backend.restrictionMessage; color: "#d8b98a" } } }
        }

        GridView {
            id: grid
            Layout.fillWidth: true
            Layout.fillHeight: true
            cellWidth: 310
            cellHeight: 190
            clip: true
            model: backend.connections
            delegate: ConnectionTile { width: 288; height: 168; title: connectionName; subtitle: description; badge: protocol; onActivated: backend.launch(index) }
            Label { anchors.centerIn: parent; visible: grid.count===0&&!backend.busy; text: "No remote systems have been assigned to you."; color: "#9fb0c8"; font.pixelSize: 18 }
        }
        BusyIndicator { running: backend.busy; visible: running; Layout.alignment: Qt.AlignHCenter }
    }

    Dialog {
        id: profileDialog
        anchors.centerIn: parent
        modal: true
        width: 560
        padding: 26
        closePolicy: Popup.CloseOnEscape
        background: Rectangle { color: "#101e30"; radius: 16; border.color: "#304b67" }
        contentItem: ColumnLayout {
            spacing: 13
            Label { text: "Edit profile"; color: "#f5f9ff"; font.pixelSize: 25; font.weight: Font.DemiBold }
            Label { text: "Update your ThinPi sign-in details."; color: "#8fa5be" }
            Label { text: "Username"; color: "#b9c8d9" }
            ThinTextField { id: profileUsername; Layout.fillWidth: true }
            Label { text: "Display name"; color: "#b9c8d9" }
            ThinTextField { id: profileDisplayName; Layout.fillWidth: true }
            Label { text: "Current password"; color: "#b9c8d9" }
            PasswordField { id: currentPassword; Layout.fillWidth: true; placeholderText: "Required to save" }
            Label { text: "New password"; color: "#b9c8d9" }
            PasswordField { id: newPassword; Layout.fillWidth: true; placeholderText: "Leave blank to keep current" }
            Label { text: "New passwords must contain at least 8 characters."; color: "#70859d"; font.pixelSize: 12 }
            RowLayout { Layout.alignment: Qt.AlignRight; ThinButton { text: "Cancel"; onClicked: profileDialog.close() } ThinButton { text: backend.busy ? "Saving…" : "Save profile"; variant: "primary"; enabled: !backend.busy; onClicked: backend.updateProfile(profileUsername.text,profileDisplayName.text,currentPassword.text,newPassword.text) } }
        }
    }

    Connections { target: backend; function onProfileUpdated() { profileDialog.close(); currentPassword.text=""; newPassword.text="" } }

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
            Label { Layout.fillWidth: true; text: "The ThinPi app will sign out and switch to this device's administrator console.\nType exit when maintenance is complete to return to the locked launcher."; color: "#c8d6e8"; wrapMode: Text.Wrap }
            RowLayout { Layout.alignment: Qt.AlignRight; ThinButton { text: "Cancel"; onClicked: maintenanceConfirm.close() } ThinButton { text: "Open console"; variant: "primary"; onClicked: { maintenanceConfirm.close(); backend.openMaintenance() } } }
        }
    }
}
