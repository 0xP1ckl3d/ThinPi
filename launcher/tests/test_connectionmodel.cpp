#include <QtTest>
#include "connectionmodel.h"

class ConnectionModelTest : public QObject {
    Q_OBJECT
private slots:
    void authorisedItemsOnly() {
        ConnectionModel model;
        model.replace(QJsonArray{
            QJsonObject{{"id", 1}, {"name", "School PC"}, {"protocol", "rdp"}},
            QJsonObject{{"id", 2}, {"name", "Gaming PC"}, {"protocol", "moonlight"}}
        });
        QCOMPARE(model.rowCount(), 2);
        QCOMPARE(model.data(model.index(0), ConnectionModel::NameRole).toString(), QString("School PC"));
        QCOMPARE(model.data(model.index(1), ConnectionModel::ProtocolRole).toString(), QString("MOONLIGHT"));
        model.clear();
        QCOMPARE(model.rowCount(), 0);
    }
};

QTEST_MAIN(ConnectionModelTest)
#include "test_connectionmodel.moc"
