import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Rectangle {
    color: "#03080f"
    ColumnLayout { visible: backend.busy || !backend.devMode; anchors.centerIn: parent; spacing: 20
        BusyIndicator { running: true; Layout.alignment: Qt.AlignHCenter; implicitWidth: 64; implicitHeight: 64 }
        Label { text: backend.sessionMessage; color: "white"; font.pixelSize: 28; Layout.alignment: Qt.AlignHCenter }
        Label { text: "The dashboard will return when the remote session ends."; color: "#9fb0c8"; font.pixelSize: 16; Layout.alignment: Qt.AlignHCenter }
        Button { text: "End session"; Layout.alignment: Qt.AlignHCenter; onClicked: backend.endSession() }
    }
    Rectangle { visible: backend.devMode && !backend.busy; anchors.fill: parent; color: "#18324b"
        Rectangle { anchors.fill: parent; gradient: Gradient { GradientStop { position: 0; color: "#174862" } GradientStop { position: 1; color: "#14243d" } } }
        Row { anchors.left: parent.left; anchors.top: parent.top; anchors.margins: 26; spacing: 18
            Repeater { model: [{icon:"⌂",name:"Home"},{icon:"▣",name:"Files"},{icon:"◉",name:"Browser"}]; delegate: Column { spacing: 5; Rectangle { width: 52; height: 52; radius: 10; color: "#ffffff22"; Label { anchors.centerIn: parent; text: modelData.icon; color: "white"; font.pixelSize: 25 } } Label { anchors.horizontalCenter: parent.horizontalCenter; text: modelData.name; color: "white"; font.pixelSize: 12 } } }
        }
        Rectangle { anchors.centerIn: parent; width: Math.min(parent.width*0.68,760); height: Math.min(parent.height*0.62,430); radius: 10; color: "#f4f6f8"; border.color: "#0a1623"; border.width: 2
            Rectangle { id: titlebar; width: parent.width; height: 42; color: "#26384c"; radius: 8
                Label { anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter; anchors.leftMargin: 16; text: "ThinPi demo desktop"; color: "white"; font.bold: true }
                Row { anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter; anchors.rightMargin: 12; spacing: 8; Repeater { model: ["—","□","×"]; delegate: Label { text: modelData; color: "#dce7f2"; font.pixelSize: 17 } } }
            }
            ColumnLayout { anchors.fill: parent; anchors.topMargin: 58; anchors.margins: 24; spacing: 15
                Label { text: "Remote session is working"; color: "#132235"; font.pixelSize: 27; font.bold: true }
                Label { text: backend.sessionMessage; color: "#34506a"; font.pixelSize: 17 }
                Rectangle { Layout.fillWidth: true; Layout.fillHeight: true; radius: 8; color: "#e7edf2"; ColumnLayout { anchors.centerIn: parent; spacing: 8; Label { text: "This safe local desktop proves login, permissions, credential resolution, ticket redemption and session lifecycle."; color: "#344a5f"; wrapMode: Text.Wrap; horizontalAlignment: Text.AlignHCenter; Layout.maximumWidth: 520 } Label { text: "No external computer is contacted in demo mode."; color: "#6c7f91"; font.pixelSize: 12; Layout.alignment: Qt.AlignHCenter } } }
            }
        }
        Rectangle { anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom; height: 54; color: "#091421e8"; Label { anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter; anchors.leftMargin: 18; text: "◉  Applications"; color: "white"; font.bold: true } Label { anchors.centerIn: parent; text: "DEMO SESSION"; color: "#68e0c5"; font.bold: true } Button { anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter; anchors.rightMargin: 12; text: "End session"; onClicked: backend.endSession() } }
    }
}
