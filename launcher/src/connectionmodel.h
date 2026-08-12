#pragma once

#include <QAbstractListModel>
#include <QJsonArray>

class ConnectionModel final : public QAbstractListModel {
    Q_OBJECT
public:
    enum Role { IdRole = Qt::UserRole + 1, NameRole, DescriptionRole, ProtocolRole, IconRole };
    explicit ConnectionModel(QObject *parent = nullptr);
    int rowCount(const QModelIndex &parent = {}) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    void replace(const QJsonArray &items);
    void clear();
    qint64 idAt(int row) const;
    QString nameAt(int row) const;
    QString protocolAt(int row) const;
    int indexOfId(qint64 id) const;
private:
    struct Item { qint64 id; QString name, description, protocol, icon; };
    QList<Item> m_items;
};
