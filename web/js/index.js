'use strict';

const ErrorTracker = {
    config: {
        endpoint: '',
        maxErrors: 10,
        timeWindow: 60000, // 1 minute
    },

    errorCount: 0,
    lastReset: Date.now(),

    init(customConfig = {}) {
        this.config = { ...this.config, ...customConfig };
        this.setupHandlers();
    },

    setupHandlers() {
        window.onerror = (msg, url, line, col, error) => {
            // If error object exists, use it directly
            // Otherwise, create a proper Error object (not a plain object)
            const errorObj = error || new Error(msg);

            // Add location info if we don't have an error object
            if (!error) {
                errorObj.filename = url;
                errorObj.lineno = line;
                errorObj.colno = col;
            }

            this.trackError(errorObj);
        };

        window.addEventListener('unhandledrejection', (event) => {
            // Handle cases where reason might not be an Error object
            const error = event.reason instanceof Error
                ? event.reason
                : new Error(String(event.reason));
            this.trackError(error);
        });
    },

    trackError(error) {
        const now = Date.now();

        if (now - this.lastReset > this.config.timeWindow) {
            this.errorCount = 0;
            this.lastReset = now;
        }

        if (this.errorCount >= this.config.maxErrors) {
            console.debug('Throttling error reporting');
            return;
        }

        this.errorCount++;

        const errorData = {
            message: error.message || String(error),
            stack: error.stack || new Error().stack, // Fallback to current stack
            url: window.location.href,
            userAgent: navigator.userAgent,
            timestamp: new Date().toISOString(),
            // Add these for better debugging
            filename: error.filename || error.fileName,
            line: error.lineno || error.lineNumber,
            column: error.colno || error.columnNumber,
        };

        fetch(this.config.endpoint, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(errorData),
            // Add keepalive to ensure errors are sent even during page unload
            keepalive: true,
        }).catch(console.error);
    }
};

const demoWidget = {
    _clearTimeout: null,

    resetCaptcha() {
        let autoWidget = window.privateCaptcha.autoWidget;
        if (autoWidget) {
            autoWidget.reset();
        }
    },

    onDifficultyChange(endpoint) {
        if (this._clearTimeout) { clearTimeout(this._clearTimeout); }

        let autoWidget = window.privateCaptcha.autoWidget;
        if (autoWidget) {
            autoWidget.reset({ puzzleEndpoint: endpoint });
        }
    },

    onCaptchaReset() {
        this._clearTimeout = setTimeout(this.resetCaptcha, 2000 /*millis*/);
    },
};

function loadScript(url, callback) {
    const scripts = document.getElementsByTagName('script');
    for (let i = 0; i < scripts.length; i++) {
        if (scripts[i].src === url) {
            if (callback) {
                setTimeout(callback, 0);
            }
            return;
        }
    }

    let script = document.createElement('script');
    script.type = 'text/javascript';
    script.src = url;
    // several events for cross browser compatibility.
    script.onreadystatechange = callback;
    script.onload = callback;
    // Fire the loading
    document.head.appendChild(script);
}

