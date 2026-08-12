#include "connectionmodel.h"
#include <QJsonObject>

ConnectionModel::ConnectionModel(QObject *parent) : QAbstractListModel(parent) {}
int ConnectionModel::rowCount(const QModelIndex &parent) const { return parent.isValid() ? 0 : m_items.size(); }
QVariant ConnectionModel::data(const QModelIndex &index, int role) const {
    if (!index.isValid() || index.row() < 0 || index.row() >= m_items.size()) return {};
    const auto &x = m_items.at(index.row());
    switch (role) { case IdRole:return x.id; case NameRole:return x.name; case DescriptionRole:return x.description; case ProtocolRole:return x.protocol.toUpper(); case IconRole:return x.icon; default:return {}; }
}
QHash<int,QByteArray> ConnectionModel::roleNames() const { return {{IdRole,"connectionId"},{NameRole,"connectionName"},{DescriptionRole,"description"},{ProtocolRole,"protocol"},{IconRole,"iconName"}}; }
void ConnectionModel::replace(const QJsonArray &items) { beginResetModel();m_items.clear();for(const auto &value:items){const auto o=value.toObject();m_items.append({o["id"].toInteger(),o["name"].toString(),o["description"].toString(),o["protocol"].toString(),o["icon"].toString()});}endResetModel(); }
void ConnectionModel::clear(){beginResetModel();m_items.clear();endResetModel();}
qint64 ConnectionModel::idAt(int row) const{return row>=0&&row<m_items.size()?m_items.at(row).id:0;}
QString ConnectionModel::nameAt(int row) const{return row>=0&&row<m_items.size()?m_items.at(row).name:QString();}
QString ConnectionModel::protocolAt(int row) const{return row>=0&&row<m_items.size()?m_items.at(row).protocol:QString();}
