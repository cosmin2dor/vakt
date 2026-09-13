# **Product Requirement Document (PRD)**

## **Project Name: Vakt**

**Product Vision:** A file-reactive household task and automation orchestrator that operates without a conventional database. Vakt uses a directory of plain-text Markdown files (a "Vault") as its single source of truth—reading user directives, scheduling reminders, tracking execution and fulfillment logs, and triggering external smart home actions.

## **1\. Executive Summary & Core Philosophy**

**Vakt** bridges manual task tracking and smart home automation. It enables users and automated systems to manage recurring routines, one-time reminders, and household events using plain-text files.

### **Core Product Principles:**

* **Plain-Text Primary:** All schedules, task states, historical logs, and metadata live strictly as plain text within Markdown files (.md).  
* **Folder-Based Organization (Vault):** Users structure their tasks across nested directories and files (e.g., /Household/Routines.md, /Meals/Plan.md).  
* **Decoupled Execution & Fulfillment:** *Sending a trigger/notification* is explicitly separated from *recording that a real-world task was fulfilled*. The system tracks both events independently without assuming one automatically dictates the other.  
* **Instant Reactivity:** Any edit to a Markdown file—made via the Vakt interface, a mobile text editor, or an automated script—instantly updates the system's runtime schedule.  
* **Stateless Delegation:** Vakt manages timing, state, and rules, delegating physical execution (e.g., flashing lights, sending push notifications) to external receiver modules.

## **2\. What Fulfillment Is Used For**

In Vakt, **Fulfillment** represents an explicit record that a real-world action or physical obligation was completed (e.g., the dog was fed, the filter was changed, or a water valve was turned off).  
Tracking fulfillment separately from system execution serves three primary product purposes:

> 1. **Closed-Loop Verification:** Enables humans or smart devices to acknowledge that an event actually happened, giving users confidence in household state.  
> 2. **Flexible Completion Workflows:** Allows a task to be marked fulfilled **before** a scheduled reminder fires (early completion), **after** a reminder fires, or **completely out-of-band** by an automated sensor or callback.  
> 3. **Auditability & History Logging:** Preserves a clean record of when reminders were dispatched (@last\_triggered) versus when the physical action was confirmed done (@last\_completed), enabling historical logging without locking the system into rigid execution assumptions.

## **3\. Data Schema & Directive Syntax**

Tasks and events are defined as standard Markdown list items enriched with inline @-directives.

### **3.1 Task Line Formatting**

\- \[ \] Task Title @id(string) @schedule(cron) | @once(timestamp) @state(enum) @target(string) \[options...\]

### **3.2 Supported Directives Reference**

| Directive | Data Type | Required? | Description |
| :---- | :---- | :---- | :---- |
| **@id** | Text String | **Yes** | A unique name/identifier for the task line (e.g., @id(dog\_feed)). |
| **@state** | Enum | **Yes** | Operational mode: active, triggered, paused, completed, or failed. |
| **@schedule** | Cron Schedule | Core | Standard 5-part timing expression. Supports multiple runs per day. |
| **@once** | Timestamp | Core | One-off target date/time in standard ISO format (YYYY-MM-DDTHH:mm:ssZ). |
| **@target** | Text String | Core | Destination integration module for the action payload (e.g., ios\_notifications). |
| **@payload** | JSON / Text | Optional | Custom payload override for the integration module, supporting {{variables}}. |
| **@skip\_count** | Whole Number | Optional | Number of upcoming schedule triggers to bypass before running again. |
| **@skip\_until** | Date/Time | Optional | Target date or timestamp until which all schedule triggers are suppressed. |
| **@last\_triggered** | Timestamp | Auto | Recorded by the system whenever a trigger signal is dispatched. |
| **@last\_completed** | Timestamp | Auto | Recorded whenever a human or automated system confirms task fulfillment. |
| **@reason** | Text String | Optional | Notes describing why a task was paused, skipped, or failed. |

## **4\. Target Integrations & Payload Dispatch**

To support a growing ecosystem, Vakt routes all outbound executions through **Integration Modules**. Targets are handled by independent, pluggable provider modules rather than static webhooks.

### **4.1 Modular Architecture & MVP Scope**

* **Pluggable System:** The system is architected to allow custom target modules. A module receives the task context and the optional @payload directive, formatting it for the provider's specific API.  
* **MVP Scope Lock:** The MVP will ship with exactly **one** native integration module: ios\_notifications. All other providers are deferred to post-MVP.

### **4.2 Payload Processing & Customization**

When a schedule slot fires, the ios\_notifications module processes the task:

> 1. **Default Output:** If no @payload is provided, the module constructs a default push notification (e.g., Title: "Task Name", Body: "Due from /Folder/File.md").  
> 2. **Custom Payload Override:** If a @payload(...) directive is present, the module parses it, interpolates runtime variables (e.g., {{title}}, {{id}}, {{file\_path}}, {{triggered\_at}}), and formats the final outbound push notification accordingly.

## **5\. Event Evaluation & System Mechanics**

### **5.1 Schedule & Log Evaluation**

* **Trigger Dispatch:** When a schedule point arrives and the entry is @state(active), Vakt hands off to the target integration module and logs @last\_triggered(NOW).  
* **Fulfillment Processing:**  
  * **One-Time (@once):** When fulfilled, the check box switches to \- \[x\] and state updates to @state(completed).  
  * **Recurring (@schedule):** When fulfilled, @last\_completed(NOW) is logged. The schedule remains intact for future cycles.  
* **Early Fulfillment:** If @last\_completed is logged prior to a scheduled trigger time, the upcoming trigger is bypassed.

### **5.2 Temporary Overrides (Skip Logic)**

* **@skip\_count(N):** Bypasses the next *N* occurrences. Decrements by 1 at each bypassed schedule interval until it reaches 0\.  
* **@skip\_until(Date):** Suppresses triggers until current time exceeds the defined date.

### **5.3 System States Reference**

| @state | System Behavior |
| :---- | :---- |
| **active** | Evaluated against current time; dispatches triggers when due. |
| **triggered** | Outbound signal dispatched; pending fulfillment or next cycle. |
| **paused** | Bypassed by scheduler evaluation. Manually suspended. |
| **completed** | Bypassed by scheduler evaluation. One-time task marked finished. |
| **failed** | Bypassed until cleared. Integration module delivery failed. |

## **6\. User Interface (PWA) & Smart Editor**

The frontend application serves as the primary touchpoint, blending a visual dashboard with a frictionless plain-text editing experience.

### **6.1 Dashboard & Browsing**

* **Aggregated Feed:** Displays active, upcoming, and triggered tasks across all Markdown files, ordered by due date and status.  
* **Quick Actions:** One-tap controls on task cards for Quick Fulfill, Pause/Resume, and Skip Next.  
* **Vault Directory:** Allows browsing the folder hierarchy to view or edit specific .md files.

### **6.2 Inline Smart Editor & Directive Helpers**

To keep the MVP minimal but mobile-friendly, the system relies on an enhanced plain-text editor rather than complex UI forms. The user authors plain Markdown directly, assisted by UI helpers.

* **@ Autocomplete:** Typing an @ character invokes an inline dropdown menu of valid directives (e.g., @id, @schedule, @once).  
* **Contextual UI Helpers:** Selecting a directive opens a specific UI helper to format the parameter effortlessly:  
  * **@once / @skip\_until:** Opens a native Date & Time Picker modal.  
  * **@target / @state:** Opens a dropdown of valid enum values.  
  * **@schedule:** Opens a popover with common cron shortcuts (Daily, Weekly) or accepts raw input.  
* **Mobile Quick-Access Bar:** A sticky accessory bar above the mobile keyboard provides immediate access to commonly used markdown characters and directives (\[ \], @, @once, @schedule, @target), circumventing mobile keyboard layout switching.

## **7\. Functional Requirements Matrix**

### **7.1 Daemon & Processing Engine**

* **FR-1.1:** Parse all Markdown files recursively within a root Vault directory.  
* **FR-1.2:** Detect file system modifications instantly and update scheduled jobs without system restarts.  
* **FR-1.3:** Evaluate fulfillment timestamps against schedule windows to handle early completions and skip logic.  
* **FR-1.4:** Route outbound executions through isolated integration modules.  
* **FR-1.5:** Write state updates (@last\_triggered, @last\_completed, @state) back to the .md file safely.

### **7.2 Application API Layer**

* **FR-2.1:** Provide endpoints to fetch, create, update, trigger, and fulfill tasks.  
* **FR-2.2:** Expose a real-time event stream to push file modification events to connected PWAs.  
* **FR-2.3:** Provide directory inspection endpoints to navigate the Vault structure.

### **7.3 User Interface (PWA)**

* **FR-3.1:** Display a unified chronological feed of tasks across the Vault.  
* **FR-3.2:** Provide instant UI state updates when interacting with task cards.  
* **FR-3.3:** Implement the Inline Smart Editor with @ autocomplete and contextual parameter helpers.  
* **FR-3.4:** Provide a Mobile Quick-Access Keyboard bar for Markdown/Directive shortcuts.

## **8\. Operational Boundaries & Non-Goals (Out of Scope for MVP)**

* **Additional Integrations:** Home Assistant, Slack, MQTT, or standard Webhooks are deferred. ios\_notifications is the sole MVP target.  
* **External Calendar Sync:** Importing/exporting .ics files or Google Calendar integration.  
* **Visual Automation Flow Builders:** No drag-and-drop workflow canvas.  
* **User Authentication:** Assumes deployment on a private, secured home network without multi-user roles.

## **9\. Open Questions & Unresolved Architecture Gaps**

The following items are identified as product gaps but are explicitly deferred from the current resolution cycle. They must be addressed before final production rollout:

> 1. **Error Recovery & Retry Policies:**  
   * If the ios\_notifications module fails to deliver (e.g., network drop, Apple Push Notification service outage), how does the system recover?  
   * Is there an automated exponential backoff mechanism, or does the task remain stuck in @state(failed) requiring manual UI intervention?  
> 2. **File Conflict Resolution (Concurrency):**  
   * Because the Vault is plain text, multiple actors can modify it simultaneously.  
   * If a user is actively typing in a .md file via the Smart Editor, and the Vakt background engine attempts to write a @last\_triggered timestamp to the same file at the exact same millisecond, what are the lock mechanisms and conflict resolution rules to prevent data loss or file corruption?