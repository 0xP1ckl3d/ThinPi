#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QSocketNotifier>
#include "backend.h"

#ifdef Q_OS_UNIX
#include <csignal>
#include <unistd.h>

namespace {
int lockSignalPipe[2]={-1,-1};
void requestKioskLock(int){const char byte=1;if(lockSignalPipe[1]>=0)(void)::write(lockSignalPipe[1],&byte,1);}
}
#endif

int main(int argc,char *argv[]){QGuiApplication app(argc,argv);QGuiApplication::setApplicationName("ThinPi");QGuiApplication::setOrganizationName("ThinPi");Backend backend;
#ifdef Q_OS_UNIX
if(::pipe(lockSignalPipe)==0){auto *lockNotifier=new QSocketNotifier(lockSignalPipe[0],QSocketNotifier::Read,&app);QObject::connect(lockNotifier,&QSocketNotifier::activated,&backend,[&backend](){char byte;(void)::read(lockSignalPipe[0],&byte,1);backend.lockKiosk();});struct sigaction action{};action.sa_handler=requestKioskLock;sigemptyset(&action.sa_mask);action.sa_flags=SA_RESTART;sigaction(SIGUSR1,&action,nullptr);}
#endif
QQmlApplicationEngine engine;engine.rootContext()->setContextProperty("backend",&backend);QObject::connect(&engine,&QQmlApplicationEngine::objectCreationFailed,&app,[](){QCoreApplication::exit(1);},Qt::QueuedConnection);engine.load(QUrl(QStringLiteral("qrc:/ThinPi/qml/Main.qml")));return app.exec();}
