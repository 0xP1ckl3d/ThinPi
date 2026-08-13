#include "backend.h"
#include <QClipboard>
#include <QCoreApplication>
#include <QCursor>
#include <QDesktopServices>
#include <QDir>
#include <QEvent>
#include <QGuiApplication>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLocalSocket>
#include <QLoggingCategory>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QProcess>
#include <QProcessEnvironment>
#include <QScreen>
#include <QStandardPaths>
#include <QUrl>
#include <QUuid>
#include <QtGlobal>

Q_LOGGING_CATEGORY(uiLog, "thinpi.launcher")

Backend::Backend(QObject *parent) : QObject(parent) {
  const auto env = QProcessEnvironment::systemEnvironment();
  m_devMode = env.value("THINPI_DEV_MODE") == "1";
  m_apiUrl = env.value("THINPI_API_URL", m_devMode ? "http://127.0.0.1:8080"
                                                   : "https://thinpi.internal");
  m_agentSocket = env.value("THINPI_AGENT_SOCKET", "/run/thinpi/agent.sock");
  m_deviceIdentifier =
      env.value("THINPI_DEVICE_ID", m_devMode ? "dev-device" : "");
  m_poll.setInterval(800);
  m_idle.setSingleShot(true);
  m_keepalive.setInterval(60000);
  m_configurationPoll.setInterval(60000);
  connect(&m_poll, &QTimer::timeout, this, &Backend::pollAgent);
  connect(&m_idle, &QTimer::timeout, this, &Backend::logout);
  connect(&m_keepalive, &QTimer::timeout, this, &Backend::keepSessionAlive);
  connect(&m_configurationPoll, &QTimer::timeout, this,
          &Backend::loadLoginUsers);
  connect(QGuiApplication::clipboard(), &QClipboard::dataChanged, this,
          &Backend::retainClipboard);
  m_configurationPoll.start();
  QCoreApplication::instance()->installEventFilter(this);
  configureScreenSleep(false);
  loadLoginUsers();
  if (m_deviceIdentifier.isEmpty())
    agentRequest({{"action", "status"}}, [this](QJsonObject x) {
      m_deviceIdentifier = x["device_identifier"].toString();
    });
}
bool Backend::eventFilter(QObject *watched, QEvent *event) {
  if (!m_token.isEmpty() && m_view == "dashboard" &&
      (event->type() == QEvent::KeyPress ||
       event->type() == QEvent::MouseButtonPress ||
       event->type() == QEvent::TouchBegin ||
       event->type() == QEvent::ApplicationActivate ||
       event->type() == QEvent::WindowActivate)) {
    armIdleLock();
    if (!m_keepalive.isActive()) {
      keepSessionAlive();
      m_keepalive.start();
    }
  }
  return QObject::eventFilter(watched, event);
}
void Backend::armIdleLock() {
  if (!m_token.isEmpty() && !m_sessionActive && m_view != "session" &&
      m_idleMinutes > 0)
    m_idle.start(m_idleMinutes * 60000);
}
void Backend::setSessionActive(bool active) {
  configureScreenSleep(active);
  if (m_sessionActive == active)
    return;
  m_sessionActive = active;
  emit sessionActiveChanged();
}
void Backend::setSessionMinimized(bool minimized) {
  if (m_sessionMinimized == minimized)
    return;
  m_sessionMinimized = minimized;
  emit sessionMinimizedChanged();
}
bool Backend::currentSessionVisible() const {
  const auto session = m_sessions.constFind(m_activeConnectionID);
  return session != m_sessions.constEnd() && session->state == "active" &&
         !session->minimized;
}
QString Backend::connectionSessionState(qint64 connectionID) const {
  const auto session = m_sessions.constFind(connectionID);
  if (session == m_sessions.constEnd())
    return {};
  return session->minimized ? QStringLiteral("minimized") : session->state;
}
bool Backend::pointerAtScreenTop() const {
  const auto position = QCursor::pos();
  const auto *screen = QGuiApplication::screenAt(position);
  return screen && position.y() <= screen->geometry().top() + 1;
}
int Backend::pointerX() const { return QCursor::pos().x(); }
bool Backend::pointerOverToolbar(int left, int width, int height) const {
  const auto position = QCursor::pos();
  return position.x() >= left && position.x() < left + width &&
         position.y() >= 0 && position.y() < height;
}
void Backend::beginToolbarInteraction() {
  if (!m_toolbarCursorOverride) {
    QGuiApplication::setOverrideCursor(Qt::ArrowCursor);
    m_toolbarCursorOverride = true;
  }
  const auto xdotool = QStandardPaths::findExecutable("xdotool");
  if (xdotool.isEmpty())
    return;
  QProcess process;
  process.start(xdotool, {QStringLiteral("getactivewindow")});
  if (!process.waitForFinished(250))
    return;
  bool valid = false;
  const auto window = QString::fromUtf8(process.readAllStandardOutput()).trimmed();
  window.toULongLong(&valid);
  if (valid)
    m_toolbarReturnWindow = window;
}
void Backend::endToolbarInteraction() {
  if (m_toolbarCursorOverride) {
    QGuiApplication::restoreOverrideCursor();
    m_toolbarCursorOverride = false;
  }
  if (m_toolbarReturnWindow.isEmpty())
    return;
  const auto xdotool = QStandardPaths::findExecutable("xdotool");
  const auto window = m_toolbarReturnWindow;
  m_toolbarReturnWindow.clear();
  if (xdotool.isEmpty())
    return;
  QProcess process;
  process.start(
      xdotool,
      {QStringLiteral("windowactivate"), QStringLiteral("--sync"), window});
  if (!process.waitForFinished(500)) {
    process.kill();
    process.waitForFinished(100);
  }
}
void Backend::configureScreenSleep(bool sessionActive) {
  if (m_devMode)
    return;
  if (sessionActive || m_screenSleepMinutes == 0) {
    QProcess::execute(QStringLiteral("xset"), {QStringLiteral("-dpms")});
    return;
  }
  const auto seconds = QString::number(m_screenSleepMinutes * 60);
  QProcess::execute(QStringLiteral("xset"),
                    {QStringLiteral("dpms"), QStringLiteral("0"),
                     QStringLiteral("0"), seconds});
  QProcess::execute(QStringLiteral("xset"), {QStringLiteral("+dpms")});
}
void Backend::clearLocalSession() {
  m_token.clear();
  m_username.clear();
  m_displayName.clear();
  m_profilePhotoUrl.clear();
  m_isAdmin = false;
  m_sessionExpired = false;
  m_connections.clear();
  m_restrictionMessage.clear();
  m_poll.stop();
  m_idle.stop();
  m_keepalive.stop();
  m_sessions.clear();
  ++m_sessionRevision;
  emit sessionsChanged();
  clearClipboard();
  setSessionActive(false);
  setSessionMinimized(false);
  if (m_activeConnectionID != 0) {
    m_activeConnectionID = 0;
    emit activeConnectionIDChanged();
  }
  emit restrictionMessageChanged();
  emit isAdminChanged();
  emit usernameChanged();
  emit displayNameChanged();
  emit profilePhotoUrlChanged();
  setView("login");
  loadLoginUsers();
}
void Backend::setView(QString x) {
  if (m_view == x)
    return;
  qCInfo(uiLog) << "view transition" << m_view << "to" << x;
  m_view = std::move(x);
  emit viewChanged();
}
void Backend::setBusy(bool x) {
  if (m_busy == x)
    return;
  m_busy = x;
  emit busyChanged();
}
void Backend::fail(const QString &message, bool offline) {
  m_error = message;
  emit errorMessageChanged();
  setBusy(false);
  if (m_sessions.isEmpty())
    m_keepalive.stop();
  setSessionActive(!m_sessions.isEmpty());
  setView(offline ? "offline" : (m_token.isEmpty() ? "login" : "dashboard"));
  if (!m_token.isEmpty() && !offline)
    armIdleLock();
}
void Backend::dismissError() {
  m_error.clear();
  emit errorMessageChanged();
}
void Backend::retry() {
  dismissError();
  if (m_token.isEmpty())
    setView("login");
  else
    loadConnections();
}
void Backend::loadLoginUsers() {
  QNetworkRequest request(QUrl(m_apiUrl + "/api/v1/login-users"));
  auto *reply = m_network.get(request);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    const auto bytes = reply->readAll();
    const auto status =
        reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    reply->deleteLater();
    if (status < 200 || status >= 300)
      return;
    const auto object = QJsonDocument::fromJson(bytes).object();
    const auto configuration = object["configuration"].toObject();
    const auto configuredSleepMinutes =
        qBound(0, configuration["screen_sleep_minutes"].toInt(15), 1440);
    if (configuredSleepMinutes != m_screenSleepMinutes) {
      m_screenSleepMinutes = configuredSleepMinutes;
      configureScreenSleep(m_sessionActive);
    }
    const auto clientTheme = configuration["client_theme"].toString("ocean");
    if (clientTheme != m_clientTheme) {
      m_clientTheme = clientTheme;
      emit clientThemeChanged();
    }
    const auto terminalTheme = configuration["terminal_theme"].toString("dark");
    if (terminalTheme != m_terminalTheme) {
      m_terminalTheme = terminalTheme;
      emit terminalThemeChanged();
    }
    auto userItems = object["items"].toArray();
    for (qsizetype i = 0; i < userItems.size(); ++i) {
      auto user = userItems[i].toObject();
      const auto photoPath = user["profile_photo_url"].toString();
      if (!photoPath.isEmpty())
        user["profile_photo_url"] =
            QUrl(m_apiUrl).resolved(QUrl(photoPath)).toString();
      userItems[i] = user;
    }
    const auto users = userItems.toVariantList();
    const auto hasMore = object["has_more"].toBool();
    if (users == m_loginUsers && hasMore == m_hasMoreUsers)
      return;
    m_loginUsers = users;
    m_hasMoreUsers = hasMore;
    emit loginUsersChanged();
  });
}
void Backend::keepSessionAlive() {
  if (m_token.isEmpty() || (m_view != "dashboard" && m_view != "session")) {
    m_keepalive.stop();
    return;
  }
  QNetworkRequest request(QUrl(m_apiUrl + "/api/v1/me"));
  request.setRawHeader("Authorization",
                       QByteArray("Bearer ") + m_token.toUtf8());
  auto *reply = m_network.get(request);
  connect(reply, &QNetworkReply::finished, this, [this, reply]() {
    const auto status =
        reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    reply->deleteLater();
    if (status == 401)
      m_sessionExpired = true;
  });
}
void Backend::controllerRequest(const QByteArray &method, const QString &path,
                                const QJsonObject &body,
                                std::function<void(QJsonObject)> ok) {
  QNetworkRequest request(QUrl(m_apiUrl + path));
  request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
  if (!m_token.isEmpty())
    request.setRawHeader("Authorization",
                         QByteArray("Bearer ") + m_token.toUtf8());
  QNetworkReply *reply = nullptr;
  const auto data = QJsonDocument(body).toJson(QJsonDocument::Compact);
  if (method == "GET")
    reply = m_network.get(request);
  else if (method == "POST")
    reply = m_network.post(request, data);
  else
    reply = m_network.sendCustomRequest(request, method, data);
  connect(
      reply, &QNetworkReply::finished, this,
      [this, reply, ok = std::move(ok)]() {
        const auto bytes = reply->readAll();
        const auto status =
            reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const auto err = reply->error();
        reply->deleteLater();
        QJsonParseError pe;
        const auto doc = QJsonDocument::fromJson(bytes, &pe);
        if (err != QNetworkReply::NoError || status < 200 || status >= 300) {
          QString message = "ThinPi Controller is unavailable. Check the "
                            "network and try again.";
          bool offline = status == 0;
          if (doc.isObject()) {
            const auto safe =
                doc.object()["error"].toObject()["message"].toString();
            if (!safe.isEmpty()) {
              message = safe;
              offline = false;
            }
          }
          if (status == 401)
            clearLocalSession();
          fail(message, offline);
          return;
        }
        ok(doc.object());
      });
}
void Backend::login(const QString &username, const QString &password) {
  if (username.trimmed().isEmpty() || password.isEmpty()) {
    fail("Enter your username and password.");
    return;
  }
  dismissError();
  setBusy(true);
  controllerRequest("POST", "/api/v1/auth/login",
                    {{"username", username.trimmed()}, {"password", password}},
                    [this](QJsonObject x) {
                      m_token = x["token"].toString();
                      const auto user = x["user"].toObject();
                      m_username = user["username"].toString();
                      m_displayName = user["display_name"].toString();
                      m_profilePhotoUrl =
                          QUrl(m_apiUrl)
                              .resolved(
                                  QUrl("/api/v1/profile-photos/" +
                                       QString::number(user["id"].toInteger())))
                              .toString();
                      m_isAdmin = user["is_admin"].toBool();
                      emit usernameChanged();
                      emit displayNameChanged();
                      emit profilePhotoUrlChanged();
                      emit isAdminChanged();
                      loadConnections();
                    });
}
void Backend::loadConnections() {
  setBusy(true);
  controllerRequest("GET", "/api/v1/connections", {}, [this](QJsonObject x) {
    m_connections.replace(x["items"].toArray());
    const auto policy = x["policy"].toObject();
    m_idleMinutes = qBound(1, policy["idle_logout_minutes"].toInt(30), 1440);
    const auto message = policy["allowed"].toBool(true)
                             ? QString{}
                             : policy["reason"].toString();
    if (message != m_restrictionMessage) {
      m_restrictionMessage = message;
      emit restrictionMessageChanged();
    }
    setBusy(false);
    setView("dashboard");
    armIdleLock();
    if (!m_keepalive.isActive()) {
      keepSessionAlive();
      m_keepalive.start();
    }
  });
}
void Backend::refresh() {
  if (!m_token.isEmpty())
    loadConnections();
}
void Backend::logout() {
  if (!m_sessions.isEmpty())
    agentRequest({{"action", "cancel"}}, [](QJsonObject) {});
  if (!m_token.isEmpty()) {
    QNetworkRequest request(QUrl(m_apiUrl + "/api/v1/auth/logout"));
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    request.setRawHeader("Authorization",
                         QByteArray("Bearer ") + m_token.toUtf8());
    auto *reply = m_network.post(request, QByteArray("{}"));
    connect(reply, &QNetworkReply::finished, reply, &QObject::deleteLater);
  }
  clearLocalSession();
  setBusy(false);
  dismissError();
}
void Backend::lockKiosk() {
  closeAdministrationBrowser();
  logout();
}
void Backend::closeAdministrationBrowser() {
  if (!m_adminBrowser)
    return;
  m_adminBrowser->terminate();
  if (!m_adminBrowser->waitForFinished(1000))
    m_adminBrowser->kill();
}
void Backend::updateProfile(const QString &username, const QString &displayName,
                            const QString &currentPassword,
                            const QString &newPassword) {
  if (username.trimmed().isEmpty() || displayName.trimmed().isEmpty() ||
      currentPassword.isEmpty()) {
    fail("Enter your username, display name and current password.");
    return;
  }
  dismissError();
  setBusy(true);
  controllerRequest("PUT", "/api/v1/me",
                    {{"username", username.trimmed()},
                     {"display_name", displayName.trimmed()},
                     {"current_password", currentPassword},
                     {"new_password", newPassword}},
                    [this](QJsonObject x) {
                      const auto user = x["user"].toObject();
                      m_username = user["username"].toString();
                      m_displayName = user["display_name"].toString();
                      emit usernameChanged();
                      emit displayNameChanged();
                      setBusy(false);
                      armIdleLock();
                      emit profileUpdated();
                    });
}
void Backend::openAdministration() {
  if (!m_isAdmin || m_token.isEmpty())
    return;
  dismissError();
  m_idle.stop();
  setBusy(true);
  controllerRequest("POST", "/api/v1/admin-handoff", {}, [this](QJsonObject x) {
    const QUrl url = QUrl(m_apiUrl).resolved(QUrl(x["path"].toString()));
    bool opened = false;
    if (m_devMode) {
      opened = QDesktopServices::openUrl(url);
    } else if (!m_adminBrowser) {
      const auto configured = QProcessEnvironment::systemEnvironment().value(
          "THINPI_ADMIN_BROWSER");
      QString browser = configured;
      if (browser.isEmpty()) {
        for (const auto &candidate : {QStringLiteral("chromium"),
                                      QStringLiteral("google-chrome-stable"),
                                      QStringLiteral("chromium-browser")}) {
          browser = QStandardPaths::findExecutable(candidate);
          if (!browser.isEmpty())
            break;
        }
      }
      if (!browser.isEmpty()) {
        const auto cacheRoot =
            QStandardPaths::writableLocation(QStandardPaths::CacheLocation);
        m_adminProfile = cacheRoot + QStringLiteral("/admin-") +
                         QUuid::createUuid().toString(QUuid::WithoutBraces);
        QDir().mkpath(m_adminProfile);
        m_adminBrowser = new QProcess(this);
        m_adminBrowser->setProgram(browser);
        m_adminBrowser->setArguments(
            {QStringLiteral("--start-maximized"),
             QStringLiteral("--user-data-dir=") + m_adminProfile,
             QStringLiteral("--no-first-run"),
             QStringLiteral("--no-default-browser-check"),
             QStringLiteral("--noerrdialogs"),
             QStringLiteral("--disable-session-crashed-bubble"),
             QStringLiteral("--disable-background-networking"),
             QStringLiteral("--disable-component-update"),
             QStringLiteral("--disable-sync"),
             QStringLiteral("--disable-translate"),
             QStringLiteral("--disable-pinch"),
             QStringLiteral("--disable-extensions"),
             QStringLiteral("--disable-plugins"),
             QStringLiteral("--disable-pdf-extension"),
             QStringLiteral("--disable-features=KeyboardLockAPI"),
             QStringLiteral("--password-store=basic"),
             QStringLiteral("--overscroll-history-navigation=0"),
             QStringLiteral("--app=") + url.toString()});
        connect(m_adminBrowser,
                qOverload<int, QProcess::ExitStatus>(&QProcess::finished), this,
                [this]() {
                  QDir(m_adminProfile).removeRecursively();
                  m_adminProfile.clear();
                  m_adminBrowser->deleteLater();
                  m_adminBrowser = nullptr;
                  loadLoginUsers();
                  armIdleLock();
                });
        m_adminBrowser->start();
        opened = m_adminBrowser->waitForStarted(3000);
        if (!opened) {
          QDir(m_adminProfile).removeRecursively();
          m_adminProfile.clear();
          m_adminBrowser->deleteLater();
          m_adminBrowser = nullptr;
        }
      }
    }
    setBusy(false);
    if (!opened)
      fail("The administration browser is unavailable on this ThinPi.");
  });
}
void Backend::openMaintenance() {
  if (!m_isAdmin || m_token.isEmpty() || m_deviceIdentifier.isEmpty())
    return;
  dismissError();
  setBusy(true);
  if (!m_sessions.isEmpty()) {
    agentRequest({{"action", "cancel"}}, [this](QJsonObject response) {
      if (!response["accepted"].toBool()) {
        setBusy(false);
        fail("Open connections could not be closed for local maintenance.");
        return;
      }
      waitForMaintenanceIdle(40);
    });
    return;
  }
  beginMaintenance();
}
void Backend::waitForMaintenanceIdle(int attemptsRemaining) {
  agentRequest({{"action", "status"}}, [this, attemptsRemaining](QJsonObject response) {
    const auto sessions = response["status"].toObject()["sessions"].toArray();
    if (sessions.isEmpty()) {
      m_sessions.clear();
      ++m_sessionRevision;
      emit sessionsChanged();
      setSessionActive(false);
      beginMaintenance();
      return;
    }
    if (attemptsRemaining <= 1) {
      setBusy(false);
      fail("Open connections did not close in time for local maintenance.");
      return;
    }
    QTimer::singleShot(250, this, [this, attemptsRemaining]() {
      waitForMaintenanceIdle(attemptsRemaining - 1);
    });
  });
}
void Backend::beginMaintenance() {
  controllerRequest(
      "POST", "/api/v1/maintenance",
      {{"device_identifier", m_deviceIdentifier}}, [this](QJsonObject x) {
        agentRequest({{"action", "maintenance"},
                      {"ticket", x["ticket"].toString()}},
                     [this](QJsonObject response) {
                       setBusy(false);
                       if (!response["accepted"].toBool()) {
                         const auto error = response["error"].toString();
                         fail(error.isEmpty()
                                  ? "Local maintenance could not be authorised."
                                  : error);
                         return;
                       }
                       logout();
                     });
      });
}
void Backend::retainClipboard() {
  if (m_token.isEmpty() || m_updatingClipboard)
    return;
  auto *clipboard = QGuiApplication::clipboard();
  const auto text = clipboard->text(QClipboard::Clipboard);
  if (text.isNull())
    return;
  m_retainedClipboard = text;
  m_updatingClipboard = true;
  clipboard->setText(m_retainedClipboard, QClipboard::Clipboard);
  if (clipboard->supportsSelection())
    clipboard->setText(m_retainedClipboard, QClipboard::Selection);
  m_updatingClipboard = false;
}
void Backend::clearClipboard() {
  m_retainedClipboard.clear();
  m_updatingClipboard = true;
  auto *clipboard = QGuiApplication::clipboard();
  clipboard->clear(QClipboard::Clipboard);
  if (clipboard->supportsSelection())
    clipboard->clear(QClipboard::Selection);
  m_updatingClipboard = false;
}
void Backend::launch(int row) {
  const auto id = m_connections.idAt(row);
  if (id == 0)
    return;
  auto existing = m_sessions.constFind(id);
  if (existing != m_sessions.constEnd() &&
      (existing->state == "error" || existing->state == "stopping")) {
    if (!existing->id.isEmpty())
      agentRequest({{"action", "cancel"}, {"session_id", existing->id}},
                   [](QJsonObject) {});
    m_sessions.remove(id);
    ++m_sessionRevision;
    emit sessionsChanged();
    existing = m_sessions.constEnd();
  }
  if (existing != m_sessions.constEnd()) {
    if (m_activeConnectionID != id) {
      m_activeConnectionID = id;
      emit activeConnectionIDChanged();
      ++m_sessionRevision;
      emit sessionsChanged();
    }
    m_activeName = existing->name;
    m_activeProtocol = existing->protocol;
    m_sessionMessage = m_activeName + " — " + m_activeProtocol;
    emit sessionMessageChanged();
    setSessionMinimized(existing->minimized);
    if (existing->minimized)
      resumeSession();
    return;
  }
  if (!m_restrictionMessage.isEmpty()) {
    fail(m_restrictionMessage);
    return;
  }
  if (m_deviceIdentifier.isEmpty()) {
    fail("The local ThinPi service is unavailable.");
    return;
  }
  m_activeConnectionID = id;
  emit activeConnectionIDChanged();
  m_sessionExpired = false;
  m_idle.stop();
  configureScreenSleep(true);
  m_activeName = m_connections.nameAt(row);
  m_activeProtocol = m_connections.protocolAt(row).toUpper();
  m_sessions.insert(id, SessionInfo{{}, QStringLiteral("redeeming_ticket"),
                                    m_activeName, m_activeProtocol, false});
  ++m_sessionRevision;
  emit sessionsChanged();
  setSessionActive(true);
  setSessionMinimized(false);
  m_sessionMessage = "Connecting to " + m_activeName + QStringLiteral("…");
  emit sessionMessageChanged();
  setBusy(true);
  setView("session");
  controllerRequest(
      "POST", QString("/api/v1/connections/%1/launch").arg(id),
      {{"device_identifier", m_deviceIdentifier}}, [this, id](QJsonObject x) {
        agentRequest(
            {{"action", "launch"}, {"ticket", x["ticket"].toString()}},
            [this, id](QJsonObject response) {
              if (!response["accepted"].toBool()) {
                m_sessions.remove(id);
                ++m_sessionRevision;
                emit sessionsChanged();
                setSessionActive(!m_sessions.isEmpty());
                fail("The local ThinPi service could not start the session.");
                return;
              }
              auto session = m_sessions.value(id);
              session.id = response["session_id"].toString();
              m_sessions.insert(id, session);
              ++m_sessionRevision;
              emit sessionsChanged();
              setBusy(false);
              m_poll.start();
              pollAgent();
            });
      });
}
void Backend::agentRequest(const QJsonObject &request,
                           std::function<void(QJsonObject)> complete) {
  auto *socket = new QLocalSocket(this);
  auto *timeout = new QTimer(socket);
  timeout->setSingleShot(true);
  timeout->setInterval(3000);
  connect(timeout, &QTimer::timeout, socket, [this, socket]() {
    socket->abort();
    socket->deleteLater();
    if (m_view == "session")
      fail("The local ThinPi service is unavailable.");
  });
  connect(socket, &QLocalSocket::connected, socket, [socket, request]() {
    socket->write(QJsonDocument(request).toJson(QJsonDocument::Compact) + "\n");
    socket->flush();
  });
  connect(socket, &QLocalSocket::readyRead, socket,
          [socket, timeout, complete = std::move(complete),
           buffer = QByteArray{}]() mutable {
            buffer += socket->readAll();
            if (!buffer.contains('\n'))
              return;
            const auto doc = QJsonDocument::fromJson(buffer);
            if (!doc.isObject())
              return;
            timeout->stop();
            complete(doc.object());
            socket->disconnectFromServer();
            socket->deleteLater();
          });
  socket->connectToServer(m_agentSocket, QIODevice::ReadWrite);
  timeout->start();
}
void Backend::pollAgent() {
  agentRequest({{"action", "status"}}, [this](QJsonObject response) {
    const auto status = response["status"].toObject();
    const auto reported = status["sessions"].toArray();
    QHash<qint64, SessionInfo> updated;
    for (const auto &value : reported) {
      const auto object = value.toObject();
      const auto sessionID = object["id"].toString();
      qint64 connectionID = object["connection_id"].toInteger();
      if (connectionID == 0) {
        for (auto it = m_sessions.constBegin(); it != m_sessions.constEnd(); ++it) {
          if (it->id == sessionID) {
            connectionID = it.key();
            break;
          }
        }
      }
      if (connectionID == 0)
        continue;
      auto session = m_sessions.value(connectionID);
      session.id = sessionID;
      session.state = object["state"].toString();
      session.minimized = object["minimized"].toBool();
      if (session.name.isEmpty()) {
        const auto row = m_connections.indexOfId(connectionID);
        session.name = m_connections.nameAt(row);
        session.protocol = m_connections.protocolAt(row).toUpper();
      }
      updated.insert(connectionID, session);

      if (session.state == "error") {
        const auto confirmation = object["confirmation"].toObject();
        const auto needsConfirmation =
            confirmation["kind"].toString() == "ssh_host_key_changed";
        if (needsConfirmation) {
          m_sshConfirmationSessionID = sessionID;
          if (!m_sshHostKeyConfirmation) {
            m_sshHostKeyConfirmation = true;
            emit sshHostKeyConfirmationChanged();
          }
        }
        if (connectionID == m_activeConnectionID) {
          const auto detail = object["last_error"].toString();
          m_error = detail.isEmpty()
                        ? "Unable to connect to " + session.name + "."
                        : detail;
          emit errorMessageChanged();
          setBusy(false);
          setView("dashboard");
        }
      }
    }
    for (auto it = m_sessions.constBegin(); it != m_sessions.constEnd(); ++it) {
      if (it->id.isEmpty() && !updated.contains(it.key()))
        updated.insert(it.key(), it.value());
    }
    m_sessions = std::move(updated);
    ++m_sessionRevision;
    emit sessionsChanged();
    setSessionActive(!m_sessions.isEmpty());

    const auto current = m_sessions.constFind(m_activeConnectionID);
    if (current != m_sessions.constEnd()) {
      m_activeName = current->name;
      m_activeProtocol = current->protocol;
      m_sessionMessage = m_activeName + " — " + m_activeProtocol;
      emit sessionMessageChanged();
      setSessionMinimized(current->minimized);
      if (current->minimized && m_view == "session")
        setView("dashboard");
    } else if (m_activeConnectionID != 0) {
      m_activeConnectionID = 0;
      emit activeConnectionIDChanged();
      setSessionMinimized(false);
      m_sessionMessage.clear();
      emit sessionMessageChanged();
      if (!m_sessionExpired)
        setView("dashboard");
    }

    if (!m_sessions.isEmpty()) {
      if (!m_keepalive.isActive()) {
        keepSessionAlive();
        m_keepalive.start();
      }
    } else {
      m_poll.stop();
      setSessionActive(false);
      setSessionMinimized(false);
      if (m_sessionExpired) {
        clearLocalSession();
      } else {
        setView("dashboard");
        armIdleLock();
      }
    }
  });
}
void Backend::endSession() {
  const auto remoteWindow = m_toolbarReturnWindow;
  endToolbarInteraction();
  const auto session = m_sessions.constFind(m_activeConnectionID);
  if (session == m_sessions.constEnd() || session->id.isEmpty())
    return;
  const auto xdotool = QStandardPaths::findExecutable("xdotool");
  if (!xdotool.isEmpty() && !remoteWindow.isEmpty())
    QProcess::startDetached(xdotool,
                            {QStringLiteral("windowclose"), remoteWindow});
  agentRequest({{"action", "cancel"}, {"session_id", session->id}},
               [this](QJsonObject response) {
                 if (!response["accepted"].toBool()) {
                   m_error = "The active connection could not be closed.";
                   emit errorMessageChanged();
                   return;
                 }
                 setView("dashboard");
               });
}
void Backend::minimizeSession() {
  endToolbarInteraction();
  const auto session = m_sessions.constFind(m_activeConnectionID);
  if (session == m_sessions.constEnd() || session->id.isEmpty() ||
      session->minimized || session->state != "active")
    return;
  agentRequest({{"action", "minimize"}, {"session_id", session->id}},
               [this](QJsonObject response) {
    if (!response["accepted"].toBool()) {
      m_error = "The active connection could not be minimized.";
      emit errorMessageChanged();
      return;
    }
    setSessionMinimized(true);
    auto current = m_sessions.value(m_activeConnectionID);
    current.minimized = true;
    m_sessions.insert(m_activeConnectionID, current);
    ++m_sessionRevision;
    emit sessionsChanged();
    setView("dashboard");
  });
}
void Backend::resumeSession() {
  const auto session = m_sessions.constFind(m_activeConnectionID);
  if (session == m_sessions.constEnd() || session->id.isEmpty())
    return;
  const auto connectionID = m_activeConnectionID;
  const auto sessionID = session->id;
  agentRequest({{"action", "resume"}, {"session_id", session->id}},
               [this, connectionID, sessionID](QJsonObject response) {
    if (!response["accepted"].toBool()) {
      agentRequest({{"action", "cancel"}, {"session_id", sessionID}},
                   [](QJsonObject) {});
      m_sessions.remove(connectionID);
      ++m_sessionRevision;
      emit sessionsChanged();
      setSessionActive(!m_sessions.isEmpty());
      setSessionMinimized(false);
      setView("dashboard");
      const auto row = m_connections.indexOfId(connectionID);
      if (row >= 0)
        launch(row);
      return;
    }
    auto current = m_sessions.value(connectionID);
    current.minimized = false;
    m_sessions.insert(connectionID, current);
    ++m_sessionRevision;
    emit sessionsChanged();
    if (m_activeConnectionID == connectionID) {
      setSessionMinimized(false);
      setView("session");
    }
  });
}
void Backend::resolveSSHHostKey(bool accept) {
  const auto connectionID = m_activeConnectionID;
  agentRequest({{"action", "resolve_ssh_host_key"},
                {"session_id", m_sshConfirmationSessionID},
                {"accept", accept}},
               [this, accept, connectionID](QJsonObject response) {
                 if (!response["accepted"].toBool()) {
                   fail(
                       "The SSH host-key confirmation is no longer available.");
                   return;
                 }
                 m_sshHostKeyConfirmation = false;
                 m_sshConfirmationSessionID.clear();
                 emit sshHostKeyConfirmationChanged();
                 m_sessions.remove(connectionID);
                 ++m_sessionRevision;
                 emit sessionsChanged();
                 setSessionActive(!m_sessions.isEmpty());
                 dismissError();
                 setBusy(false);
                 setView("dashboard");
                 armIdleLock();
                 if (accept)
                   launch(m_connections.indexOfId(connectionID));
               });
}
