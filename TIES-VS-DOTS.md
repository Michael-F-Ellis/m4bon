This guide summarizes the core principles of music engraving regarding the choice between ties and dotted durations. It is structured to provide an LLM with clear "if-then" logic for an automated notation system.

### **Core Principle: Visibility of the Pulse**
The most important rule in music engraving is that the **beat structure must be immediately visible**. If a duration obscures the start of a beat—particularly the "invisible barline" in the middle of a measure—it should typically be broken into tied notes rather than a single dotted note.

---

### **1. General Rules (All Time Signatures)**
*   **Across Barlines:** Always use a **tie**. A single note duration (dotted or otherwise) can never cross a barline.
*   **The "Invisible Barline" (4/4 Rule):** In 4/4 time, the start of **Beat 3** must be visible. 
    *   *Example:* A duration of 3 beats starting on Beat 2 must be written as a **quarter note tied to a half note**, not a dotted half note.
*   **Rhythmic Clarity Rule:** Use a tie if a dot would extend a note across a major subdivision of the measure.
    *   *Simple Time:* Divide the measure into halves (4/4) or quarters.
    *   *Compound Time:* Group by the pulse of the dotted beat (6/8, 9/8, 12/8).

### **2. Simple Time Signatures (2/4, 3/4, 4/4)**
*   **Dots are Preferred When:**
    *   The duration fits within a single beat (e.g., dotted 8th + 16th).
    *   The duration starts on a strong beat and does not cross the measure's midpoint. (e.g., a dotted half on Beat 1 of 4/4).
*   **Ties are Mandatory When:**
    *   **Crossing the Midpoint:** In 4/4, a duration starting on Beat 2 that lasts 1.5 beats or more must be tied at Beat 3.
    *   **Syncopation Exception:** The "quarter-half-quarter" pattern is a traditional exception in 4/4, but for modern clarity, "quarter-tied-eighths-quarter" is sometimes preferred in complex textures.
    *   **3/4 Time:** Avoid a dotted half note if you need to emphasize a specific internal beat for rhythmic complexity, though a dotted half is standard for a full measure.

### **3. Compound Time Signatures (6/8, 9/8, 12/8)**
*   **Dots are Preferred When:**
    *   Representing the main pulse. In 6/8, a **dotted quarter** is the standard beat unit.
    *   A full measure in 6/8 is a **dotted half**.
*   **Ties are Mandatory When:**
    *   **Syncopation across pulses:** If a note starts on the 2nd eighth note of a beat and lasts for 3 eighth notes, it must be written as two tied notes to show where the next dotted-quarter pulse begins.
    *   **Rests:** Never use a dotted rest to cross the pulse in compound time; use a beat-aligned rest instead.

### **4. Summary Logic for LLM Implementation**
| Scenario | Action | Reason |
| :--- | :--- | :--- |
| Duration crosses a barline | **Tie** | Mandatory engraving rule. |
| Duration crosses the midpoint of a 4/4 bar | **Tie** | "Invisible Barline" rule for readability. |
| Duration is 3 beats starting on Beat 1 of 4/4 | **Dot** (Dotted Half) | Does not obscure the midpoint. |
| Duration is 1.5 beats starting on the "and" of 1 | **Tie** (8th tied to Quarter) | Keeps Beat 2 visible. |
| Full measure in 6/8 | **Dot** (Dotted Half) | Standard for compound duple. |
| Syncopation across the 2nd pulse of 6/8 | **Tie** | Maintains the "three-eight" grouping feel. |

---

### **Source References & Further Reading**
*   **[Elaine Gould, *Behind Bars: The Definitive Guide to Music Notation*](https://www.fabermusic.com/shop/behind-bars-534)** – The gold standard for modern engraving. Chapter 2 specifically covers "Dotted notes" and "Ties."
*   **[Gardner Read, *Music Notation: A Manual of Modern Practice*](https://archive.org/details/musicnotationman0000read)** – A classic reference for traditional rhythmic grouping and tie usage.
*   **[Standard Music Theory: Dots vs. Ties (Music Stack Exchange)](https://music.stackexchange.com/questions/21980/when-to-use-a-dot-or-a-tie-in-music-notation)** – Practical discussion on the "Invisible Barline" and rhythmic clarity.
*   **[Music Theory Classroom: Dots and Ties](https://musictheory.pugetsound.edu/mt21c/DotsAndTies.html)** – Detailed breakdown of the mathematical and visual differences.
*   **[Rhythmic Grouping & Beaming Rules (Musicnotes)](https://www.musicnotes.com/now/tips/note-beaming-and-grouping-in-music-theory/)** – Explains how beaming and ties work together to define the meter.
