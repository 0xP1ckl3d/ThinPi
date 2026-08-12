#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include "backend.h"

int main(int argc,char *argv[]){QGuiApplication app(argc,argv);QGuiApplication::setApplicationName("ThinPi");QGuiApplication::setOrganizationName("ThinPi");Backend backend;QQmlApplicationEngine engine;engine.rootContext()->setContextProperty("backend",&backend);QObject::connect(&engine,&QQmlApplicationEngine::objectCreationFailed,&app,[](){QCoreApplication::exit(1);},Qt::QueuedConnection);engine.load(QUrl(QStringLiteral("qrc:/ThinPi/qml/Main.qml")));return app.exec();}
