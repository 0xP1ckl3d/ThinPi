import QtQuick
import QtQuick.Controls

Dialog { id: dialog; property string message; modal: true; anchors.centerIn: Overlay.overlay; width: 480; implicitWidth: 480; title: "ThinPi"; standardButtons: Dialog.Ok; closePolicy: Popup.NoAutoClose; contentItem: Label { text: dialog.message; color: "white"; wrapMode: Text.Wrap; padding: 18 } }
