#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QSocketNotifier>
#include "backend.h"

#ifdef Q_OS_UNIX
#include <csignal>
#include <unistd.h>

namespace {
int kioskSignalPipe[2]={-1,-1};
void requestKioskAction(int signal){const char byte=signal==SIGUSR2?2:1;if(kioskSignalPipe[1]>=0)(void)::write(kioskSignalPipe[1],&byte,1);}
}
#endif

int main(int argc,char *argv[]){QGuiApplication app(argc,argv);QGuiApplication::setApplicationName("ThinPi");QGuiApplication::setOrganizationName("ThinPi");Backend backend;
#ifdef Q_OS_UNIX
if(::pipe(kioskSignalPipe)==0){auto *notifier=new QSocketNotifier(kioskSignalPipe[0],QSocketNotifier::Read,&app);QObject::connect(notifier,&QSocketNotifier::activated,&backend,[&backend](){char byte;(void)::read(kioskSignalPipe[0],&byte,1);if(byte==2)backend.minimizeSession();else backend.lockKiosk();});struct sigaction action{};action.sa_handler=requestKioskAction;sigemptyset(&action.sa_mask);action.sa_flags=SA_RESTART;sigaction(SIGUSR1,&action,nullptr);sigaction(SIGUSR2,&action,nullptr);}
#endif
QQmlApplicationEngine engine;engine.rootContext()->setContextProperty("backend",&backend);QObject::connect(&engine,&QQmlApplicationEngine::objectCreationFailed,&app,[](){QCoreApplication::exit(1);},Qt::QueuedConnection);engine.load(QUrl(QStringLiteral("qrc:/ThinPi/qml/Main.qml")));return app.exec();}
