#pragma once

#include <QObject>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QTimer>
#include <QVariantList>
#include <functional>
#include "connectionmodel.h"
class QProcess;
class QEvent;

class Backend final : public QObject {
    Q_OBJECT
    Q_PROPERTY(QString view READ view NOTIFY viewChanged)
    Q_PROPERTY(QString displayName READ displayName NOTIFY displayNameChanged)
    Q_PROPERTY(QString username READ username NOTIFY usernameChanged)
    Q_PROPERTY(QVariantList loginUsers READ loginUsers NOTIFY loginUsersChanged)
    Q_PROPERTY(bool hasMoreUsers READ hasMoreUsers NOTIFY loginUsersChanged)
    Q_PROPERTY(QString errorMessage READ errorMessage NOTIFY errorMessageChanged)
    Q_PROPERTY(QString sessionMessage READ sessionMessage NOTIFY sessionMessageChanged)
    Q_PROPERTY(QString restrictionMessage READ restrictionMessage NOTIFY restrictionMessageChanged)
    Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
    Q_PROPERTY(bool isAdmin READ isAdmin NOTIFY isAdminChanged)
    Q_PROPERTY(bool sessionActive READ sessionActive NOTIFY sessionActiveChanged)
    Q_PROPERTY(bool devMode READ devMode CONSTANT)
    Q_PROPERTY(ConnectionModel* connections READ connections CONSTANT)
public:
    explicit Backend(QObject *parent=nullptr);
    QString view()const{return m_view;} QString displayName()const{return m_displayName;}
    QString username()const{return m_username;} QVariantList loginUsers()const{return m_loginUsers;} bool hasMoreUsers()const{return m_hasMoreUsers;}
    QString errorMessage()const{return m_error;} QString sessionMessage()const{return m_sessionMessage;}
    QString restrictionMessage()const{return m_restrictionMessage;}
    bool busy()const{return m_busy;} bool isAdmin()const{return m_isAdmin;} bool sessionActive()const{return m_sessionActive;} bool devMode()const{return m_devMode;} ConnectionModel* connections(){return &m_connections;}
    Q_INVOKABLE void login(const QString &username,const QString &password);
    Q_INVOKABLE void logout(); Q_INVOKABLE void refresh(); Q_INVOKABLE void launch(int row);
    Q_INVOKABLE void openAdministration();
    Q_INVOKABLE void openMaintenance();
    Q_INVOKABLE void updateProfile(const QString &username,const QString &displayName,const QString &currentPassword,const QString &newPassword);
    Q_INVOKABLE void endSession(); Q_INVOKABLE void dismissError(); Q_INVOKABLE void retry();
protected:
    bool eventFilter(QObject *watched,QEvent *event) override;
signals:
    void viewChanged();void displayNameChanged();void usernameChanged();void loginUsersChanged();void errorMessageChanged();void sessionMessageChanged();void busyChanged();
    void restrictionMessageChanged();void isAdminChanged();void sessionActiveChanged();void profileUpdated();
private:
    void setView(QString);void setBusy(bool);void fail(const QString&,bool offline=false);void loadConnections();
    void controllerRequest(const QByteArray &method,const QString &path,const QJsonObject &body,std::function<void(QJsonObject)> ok);
    void agentRequest(const QJsonObject &request,std::function<void(QJsonObject)> complete);
    void pollAgent();
    void armIdleLock(); void clearLocalSession(); void loadLoginUsers(); void keepSessionAlive(); void setSessionActive(bool active);
    QNetworkAccessManager m_network;ConnectionModel m_connections;QTimer m_poll,m_idle,m_keepalive;
    QString m_apiUrl,m_agentSocket,m_token,m_deviceIdentifier,m_view="login",m_username,m_displayName,m_error,m_sessionMessage,m_restrictionMessage,m_activeName,m_activeProtocol;
    QVariantList m_loginUsers;
    int m_idleMinutes=30;
    bool m_busy=false,m_isAdmin=false,m_devMode=false,m_seenActive=false,m_sessionActive=false,m_sessionExpired=false,m_hasMoreUsers=false;
    QProcess *m_adminBrowser=nullptr;
    QString m_adminProfile;
};
