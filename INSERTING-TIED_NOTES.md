To insert tied notes in a MuseScore 4 plugin, you cannot use an object method like addTie(), as it does not exist in the MuseScore 4 QML API. Instead, you must use the score execution engine to trigger a tie via the selection cursor. [1, 2] 
The specific syntax depends on whether you are writing an interactive script (manipulating what the user has currently highlighted) or a generation script (building notes sequentially from scratch).
## Method 1: Using UI Commands (Simplest)
If your plugin modifies an existing note or works through active curation, you can explicitly add or select the note and execute MuseScore’s internal tie action: [1, 3] 

// 1. Select the base note or element in your script loop
curScore.selection.select(myNote);

// 2. Execute the tie command (acts exactly like pressing 'T' in the application)
cmd("tie"); 

Note: The cmd("tie") action functions as a toggle. Executing it on a note that is already tied will remove the tie. [1] 
## Method 2: Sequential Generation (Cursor Input)
If you are generating music from scratch via a plugin cursor, the program requires you to input the starting note, switch your cursor to the desired next duration, and invoke the tie. MuseScore 4 will automatically generate the corresponding destination note and connect them: [4, 5] 

var cursor = curScore.newCursor();
cursor.rewind(0); // Move to the start of the score

// 1. Drop your initial pitch (e.g., a quarter note)
cursor.addNote(60); // Adds middle C

// 2. Select your next target duration length
// (This prepares MuseScore to build the tied extension)

// 3. Trigger the tie to dynamically insert and link the next note
cmd("tie"); 

## Critical API Structural Differences
If you are converting an older plugin from MuseScore 3, be aware of these vital object-model changes:

* No tieForward or tieBack properties: In MuseScore 3, you could read or write properties directly onto the note elements (note.tieForward = true). MuseScore 4's updated API removes these direct boolean flags for structural integrity. [1] 
* Selection Required: The cmd("tie") workflow strictly relies on focus. Ensure your cursor or selection bounds are explicitly set before calling the command, or the plugin will fail silently or tie the wrong notes. [1, 6] 

------------------------------
If you would like to expand your script, let me know:

* Are you trying to tie a complex chord, or just single notes?
* Do your tied notes cross over a bar line?
* Would you like an example of how to read and detect existing ties in a score?


[1] [https://musescore.org](https://musescore.org/en/node/381695)
[2] [https://musescore.org](https://musescore.org/en/handbook/3/ties)
[3] [https://www.youtube.com](https://www.youtube.com/watch?v=L0o_K4DWdZ8&t=6)
[4] [https://www.youtube.com](https://www.youtube.com/watch?v=L0o_K4DWdZ8&t=6)
[5] [https://musescore.org](https://musescore.org/id/print/book/export/html/36561)
[6] [https://musescore.org](https://musescore.org/en/handbook/3/ties)

