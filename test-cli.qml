import MuseScore 3.0
import QtQuick 2.9

MuseScore {
    title: "test-cli"
    description: "Minimal test for non-dialog plugin invocation"
    version: "0.1"
    categoryCode: "composing-arranging-tools"

    onRun: {
        console.log("[test-cli] onRun() called!");
        // Try to add a note to prove we have access
        if (typeof curScore !== "undefined" && curScore) {
            console.log("[test-cli] Score found!");
            try {
                var cursor = curScore.newCursor();
                cursor.track = 0;
                cursor.rewind(0);
                curScore.startCmd();
                cursor.setDuration(1, 4);
                cursor.addNote(60, false);
                curScore.endCmd();
                console.log("[test-cli] Note inserted!");
            } catch (e) {
                console.log("[test-cli] Error: " + e);
            }
        }
        Qt.quit();
    }
}
