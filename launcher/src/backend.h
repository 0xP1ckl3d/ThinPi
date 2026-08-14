#pragma once

#include "connectionmodel.h"
#include <QJsonObject>
#include <QHash>
#include <QNetworkAccessManager>
#include <QObject>
#include <QTimer>
#include <QVariantList>
#include <functional>
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
  Q_PROPERTY(
      QString sessionMessage READ sessionMessage NOTIFY sessionMessageChanged)
  Q_PROPERTY(QString restrictionMessage READ restrictionMessage NOTIFY
                 restrictionMessageChanged)
  Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
  Q_PROPERTY(bool isAdmin READ isAdmin NOTIFY isAdminChanged)
  Q_PROPERTY(bool sessionActive READ sessionActive NOTIFY sessionActiveChanged)
  Q_PROPERTY(bool sessionMinimized READ sessionMinimized NOTIFY
                 sessionMinimizedChanged)
  Q_PROPERTY(qint64 activeConnectionID READ activeConnectionID NOTIFY
                 activeConnectionIDChanged)
  Q_PROPERTY(int sessionRevision READ sessionRevision NOTIFY sessionsChanged)
  Q_PROPERTY(bool hasOpenSessions READ hasOpenSessions NOTIFY sessionsChanged)
  Q_PROPERTY(bool currentSessionVisible READ currentSessionVisible NOTIFY
                 sessionsChanged)
  Q_PROPERTY(bool sshHostKeyConfirmation READ sshHostKeyConfirmation NOTIFY
                 sshHostKeyConfirmationChanged)
  Q_PROPERTY(bool audioUnavailableConfirmation READ audioUnavailableConfirmation
                 NOTIFY audioUnavailableConfirmationChanged)
  Q_PROPERTY(QString clientTheme READ clientTheme NOTIFY clientThemeChanged)
  Q_PROPERTY(
      QString terminalTheme READ terminalTheme NOTIFY terminalThemeChanged)
  Q_PROPERTY(QString profilePhotoUrl READ profilePhotoUrl NOTIFY
                 profilePhotoUrlChanged)
  Q_PROPERTY(bool devMode READ devMode CONSTANT)
  Q_PROPERTY(ConnectionModel *connections READ connections CONSTANT)
public:
  explicit Backend(QObject *parent = nullptr);
  QString view() const { return m_view; }
  QString displayName() const { return m_displayName; }
  QString username() const { return m_username; }
  QVariantList loginUsers() const { return m_loginUsers; }
  bool hasMoreUsers() const { return m_hasMoreUsers; }
  QString errorMessage() const { return m_error; }
  QString sessionMessage() const { return m_sessionMessage; }
  QString restrictionMessage() const { return m_restrictionMessage; }
  bool busy() const { return m_busy; }
  bool isAdmin() const { return m_isAdmin; }
  bool sessionActive() const { return m_sessionActive; }
  bool sessionMinimized() const { return m_sessionMinimized; }
  qint64 activeConnectionID() const { return m_activeConnectionID; }
  int sessionRevision() const { return m_sessionRevision; }
  bool hasOpenSessions() const { return !m_sessions.isEmpty(); }
  bool currentSessionVisible() const;
  bool devMode() const { return m_devMode; }
  bool sshHostKeyConfirmation() const { return m_sshHostKeyConfirmation; }
  bool audioUnavailableConfirmation() const {
    return m_audioUnavailableConfirmation;
  }
  ConnectionModel *connections() { return &m_connections; }
  QString clientTheme() const { return m_clientTheme; }
  QString terminalTheme() const { return m_terminalTheme; }
  QString profilePhotoUrl() const { return m_profilePhotoUrl; }
  Q_INVOKABLE void login(const QString &username, const QString &password);
  Q_INVOKABLE void logout();
  Q_INVOKABLE void refresh();
  Q_INVOKABLE void launch(int row);
  Q_INVOKABLE void lockKiosk();
  Q_INVOKABLE void openAdministration();
  Q_INVOKABLE void openMaintenance();
  Q_INVOKABLE void updateProfile(const QString &username,
                                 const QString &displayName,
                                 const QString &currentPassword,
                                 const QString &newPassword);
  Q_INVOKABLE void endSession();
  Q_INVOKABLE void minimizeSession();
  Q_INVOKABLE bool pointerAtScreenTop() const;
  Q_INVOKABLE int pointerX() const;
  Q_INVOKABLE bool pointerOverToolbar(int left, int width, int height) const;
  Q_INVOKABLE void beginToolbarInteraction();
  Q_INVOKABLE void endToolbarInteraction();
  Q_INVOKABLE QString connectionSessionState(qint64 connectionID) const;
  Q_INVOKABLE void dismissError();
  Q_INVOKABLE void retry();
  Q_INVOKABLE void resolveSSHHostKey(bool accept);
  Q_INVOKABLE void resolveAudioUnavailable(bool accept);

protected:
  bool eventFilter(QObject *watched, QEvent *event) override;
signals:
  void viewChanged();
  void displayNameChanged();
  void usernameChanged();
  void loginUsersChanged();
  void errorMessageChanged();
  void sessionMessageChanged();
  void busyChanged();
  void restrictionMessageChanged();
  void isAdminChanged();
  void sessionActiveChanged();
  void sessionMinimizedChanged();
  void activeConnectionIDChanged();
  void sessionsChanged();
  void sshHostKeyConfirmationChanged();
  void audioUnavailableConfirmationChanged();
  void profileUpdated();
  void clientThemeChanged();
  void terminalThemeChanged();
  void profilePhotoUrlChanged();

private:
  void setView(QString);
  void setBusy(bool);
  void fail(const QString &, bool offline = false);
  void loadConnections();
  void controllerRequest(const QByteArray &method, const QString &path,
                         const QJsonObject &body,
                         std::function<void(QJsonObject)> ok);
  void agentRequest(const QJsonObject &request,
                    std::function<void(QJsonObject)> complete);
  void pollAgent();
  void armIdleLock();
  void clearLocalSession();
  void loadLoginUsers();
  void keepSessionAlive();
  void setSessionActive(bool active);
  void setSessionMinimized(bool minimized);
  void resumeSession();
  void beginMaintenance();
  void waitForMaintenanceIdle(int attemptsRemaining);
  void configureScreenSleep(bool sessionActive);
  void closeAdministrationBrowser();
  void retainClipboard();
  void clearClipboard();
  QNetworkAccessManager m_network;
  ConnectionModel m_connections;
  QTimer m_poll, m_idle, m_keepalive, m_configurationPoll;
  QString m_apiUrl, m_agentSocket, m_token, m_deviceIdentifier,
      m_view = "login", m_username, m_displayName, m_error, m_sessionMessage,
      m_restrictionMessage, m_activeName, m_activeProtocol,
      m_clientTheme = "ocean", m_terminalTheme = "dark", m_profilePhotoUrl,
      m_retainedClipboard;
  qint64 m_activeConnectionID = 0;
  struct SessionInfo {
    QString id, state, name, protocol;
    bool minimized = false;
  };
  QHash<qint64, SessionInfo> m_sessions;
  QString m_sshConfirmationSessionID;
  QString m_audioConfirmationSessionID;
  int m_sessionRevision = 0;
  QVariantList m_loginUsers;
  int m_idleMinutes = 30;
  int m_screenSleepMinutes = 15;
  bool m_busy = false, m_isAdmin = false, m_devMode = false,
       m_sessionActive = false, m_sessionExpired = false,
       m_hasMoreUsers = false, m_sshHostKeyConfirmation = false,
       m_audioUnavailableConfirmation = false;
  bool m_sessionMinimized = false;
  bool m_updatingClipboard = false;
  bool m_toolbarCursorOverride = false;
  QProcess *m_adminBrowser = nullptr;
  QString m_adminProfile, m_toolbarReturnWindow;
};
