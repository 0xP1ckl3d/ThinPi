cmake_minimum_required(VERSION 3.21)
project(ThinPiLauncher VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)

find_package(Qt6 6.4 REQUIRED COMPONENTS Core Gui Quick Qml Network)
qt_standard_project_setup(REQUIRES 6.4)
if(Qt6_VERSION VERSION_GREATER_EQUAL 6.5)
  qt_policy(SET QTP0004 NEW)
endif()

qt_add_executable(thinpi-launcher src/main.cpp src/backend.cpp src/connectionmodel.cpp)
target_include_directories(thinpi-launcher PRIVATE src)
target_link_libraries(thinpi-launcher PRIVATE Qt6::Core Qt6::Gui Qt6::Quick Qt6::Qml Qt6::Network)

qt_add_qml_module(thinpi-launcher
    URI ThinPi
    VERSION 1.0
    QML_FILES
        qml/Main.qml
        qml/Login.qml
        qml/Dashboard.qml
        qml/ConnectionTile.qml
        qml/ErrorDialog.qml
        qml/SessionOverlay.qml
        qml/SessionControls.qml
        qml/ThinButton.qml
        qml/ThinTextField.qml
        qml/PasswordField.qml
        qml/Theme.qml
    RESOURCES assets/thinpi.svg
)

install(TARGETS thinpi-launcher RUNTIME DESTINATION bin)

include(CTest)
if(BUILD_TESTING)
  find_package(Qt6 REQUIRED COMPONENTS Test)
  qt_add_executable(thinpi-launcher-tests tests/test_connectionmodel.cpp src/connectionmodel.cpp)
  target_include_directories(thinpi-launcher-tests PRIVATE src)
  target_link_libraries(thinpi-launcher-tests PRIVATE Qt6::Core Qt6::Test)
  add_test(NAME launcher-model COMMAND thinpi-launcher-tests)
endif()
