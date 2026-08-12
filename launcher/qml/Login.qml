import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    id: loginRoot
    color: "#07111e"
    property string selectedUsername: ""
    property string selectedDisplayName: ""
    property bool manualUsername: false

    Rectangle { anchors.fill: parent; gradient: Gradient { GradientStop { position: 0; color: "#0d2035" } GradientStop { position: 0.55; color: "#07111e" } GradientStop { position: 1; color: "#06101b" } } }
    Rectangle { width: 520; height: 520; radius: 260; color: "#0b685b22"; anchors.right: parent.right; anchors.top: parent.top; anchors.rightMargin: -170; anchors.topMargin: -250 }

    ColumnLayout {
        anchors.centerIn: parent
        width: Math.min(parent.width - 72, 780)
        spacing: 16

        Image { source: "qrc:/qt/qml/ThinPi/assets/thinpi.svg"; sourceSize.width: 72; sourceSize.height: 72; Layout.alignment: Qt.AlignHCenter }
        Label { text: "ThinPi"; color: "#f5f9ff"; font.pixelSize: 38; font.weight: Font.DemiBold; Layout.alignment: Qt.AlignHCenter }
        Label { text: loginRoot.selectedUsername ? "Welcome back" : "Who's signing in?"; color: "#93a9c1"; font.pixelSize: 17; Layout.alignment: Qt.AlignHCenter; Layout.bottomMargin: 10 }

        Flow {
            visible: !loginRoot.selectedUsername
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: Math.min(720, loginRoot.width - 72)
            spacing: 12

            Repeater {
                model: backend.loginUsers
                delegate: Button {
                    id: userCard
                    required property var modelData
                    width: 224
                    height: 108
                    text: modelData.display_name
                    onClicked: {
                        loginRoot.selectedUsername = modelData.username
                        loginRoot.selectedDisplayName = modelData.display_name
                        loginRoot.manualUsername = false
                        password.focusInput()
                    }
                    contentItem: RowLayout {
                        spacing: 13
                        Rectangle { width: 48; height: 48; radius: 15; color: "#173d4d"; Label { anchors.centerIn: parent; text: userCard.modelData.display_name.slice(0,1).toUpperCase(); color: "#68e4c7"; font.pixelSize: 22; font.bold: true } }
                        ColumnLayout { Layout.fillWidth: true; spacing: 2; Label { Layout.fillWidth: true; text: userCard.modelData.display_name; color: "#f5f9ff"; font.pixelSize: 17; font.weight: Font.DemiBold; elide: Text.ElideRight } Label { Layout.fillWidth: true; text: "@" + userCard.modelData.username; color: "#8198b1"; font.pixelSize: 12; elide: Text.ElideRight } }
                    }
                    background: Rectangle { radius: 15; color: userCard.down ? "#19344d" : userCard.hovered ? "#172d43" : "#102135"; border.width: userCard.activeFocus ? 2 : 1; border.color: userCard.activeFocus ? "#5ed9bd" : "#29435f" }
                }
            }

            Button {
                id: otherCard
                visible: backend.hasMoreUsers || backend.loginUsers.length === 0
                width: 224
                height: 108
                onClicked: { loginRoot.selectedUsername = "other"; loginRoot.selectedDisplayName = "Other user"; loginRoot.manualUsername = true; manualUser.forceActiveFocus() }
                contentItem: RowLayout { spacing: 13; Rectangle { width: 48; height: 48; radius: 15; color: "#183047"; Label { anchors.centerIn: parent; text: "…"; color: "#7fb3ff"; font.pixelSize: 24; font.bold: true } } ColumnLayout { Layout.fillWidth: true; Label { text: "Other user"; color: "#f5f9ff"; font.pixelSize: 17; font.weight: Font.DemiBold } Label { text: "Enter username"; color: "#8198b1"; font.pixelSize: 12 } } }
                background: Rectangle { radius: 15; color: otherCard.down ? "#19344d" : otherCard.hovered ? "#172d43" : "#102135"; border.width: otherCard.activeFocus ? 2 : 1; border.color: otherCard.activeFocus ? "#5ed9bd" : "#29435f" }
            }
        }

        ColumnLayout {
            visible: !!loginRoot.selectedUsername
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: 420
            spacing: 13

            RowLayout {
                Layout.fillWidth: true
                ThinButton { text: "← Back"; onClicked: { loginRoot.selectedUsername=""; loginRoot.selectedDisplayName=""; loginRoot.manualUsername=false; password.text="" } }
                Item { Layout.fillWidth: true }
                Label { text: loginRoot.selectedDisplayName; color: "#f5f9ff"; font.pixelSize: 19; font.weight: Font.DemiBold }
            }
            ThinTextField { id: manualUser; visible: loginRoot.manualUsername; Layout.fillWidth: true; placeholderText: "Username"; onAccepted: password.focusInput() }
            PasswordField { id: password; Layout.fillWidth: true; enabled: !backend.busy; placeholderText: "Password"; onAccepted: signInButton.clicked() }
            ThinButton { id: signInButton; text: backend.busy ? "Signing in…" : "Sign in"; variant: "primary"; Layout.fillWidth: true; enabled: !backend.busy; onClicked: backend.login(loginRoot.manualUsername ? manualUser.text : loginRoot.selectedUsername,password.text) }
            BusyIndicator { running: backend.busy; visible: running; Layout.alignment: Qt.AlignHCenter; palette.dark: "#5ed9bd" }
        }
    }
}
