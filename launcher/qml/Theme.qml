import QtQuick

QtObject {
    property string paletteName: "ocean"
    readonly property var colors: {
        switch (paletteName) {
        case "graphite": return {background:"#101216", backgroundAlt:"#181b21", surface:"#22262e", surfaceHover:"#2c323c", border:"#4c5563", accent:"#b8c2d1", accentText:"#111317", text:"#f5f7fa", muted:"#a7b0bd"}
        case "forest": return {background:"#07130f", backgroundAlt:"#0c2018", surface:"#123126", surfaceHover:"#194536", border:"#2c6651", accent:"#72e3ae", accentText:"#06130e", text:"#f0fff8", muted:"#9bc8b4"}
        case "sunset": return {background:"#1b0d18", backgroundAlt:"#2b1321", surface:"#3a1b2a", surfaceHover:"#512338", border:"#75405a", accent:"#ffad73", accentText:"#251006", text:"#fff4ed", muted:"#d4adbd"}
        case "high-contrast": return {background:"#000000", backgroundAlt:"#090909", surface:"#111111", surfaceHover:"#242424", border:"#ffffff", accent:"#ffff00", accentText:"#000000", text:"#ffffff", muted:"#e5e5e5"}
        default: return {background:"#07111e", backgroundAlt:"#0d2035", surface:"#12243a", surfaceHover:"#1b3551", border:"#29435f", accent:"#5ed9bd", accentText:"#06131b", text:"#f5f9ff", muted:"#93a9c1"}
        }
    }
    readonly property color background: colors.background
    readonly property color backgroundAlt: colors.backgroundAlt
    readonly property color surface: colors.surface
    readonly property color surfaceHover: colors.surfaceHover
    readonly property color border: colors.border
    readonly property color accent: colors.accent
    readonly property color accentText: colors.accentText
    readonly property color text: colors.text
    readonly property color muted: colors.muted
}
