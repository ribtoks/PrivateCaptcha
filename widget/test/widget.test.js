import test from 'node:test';
import assert from 'node:assert';
import { Window } from 'happy-dom';

const window = new Window({
    url: 'https://localhost:8080'
});

global.window = window;
global.document = window.document;
global.HTMLElement = window.HTMLElement;
global.CustomEvent = window.CustomEvent;
global.CSSStyleSheet = window.CSSStyleSheet;

// we have to mock worker too
global.Worker = class Worker {
    constructor() {
        this.onmessage = null;
        this.onerror = null;
    }

    postMessage(data) {
        setTimeout(() => {
            if (data.command === 'init') {
                this.onmessage?.({ data: { command: 'init' } });
            } else if (data.command === 'solve') {
                // Simulate immediate solution for zero puzzle
                this.onmessage?.({
                    data: {
                        command: 'solve',
                        argument: {
                            id: BigInt(data.argument.id || 0),
                            solution: new Uint8Array(8),
                            wasm: false
                        }
                    }
                });
            }
        }, 10);
    }

    terminate() { }
};


test('CaptchaWidget execute() fires finished event and callback', async (t) => {
    document.body.innerHTML = `
        <form>
            <div class="private-captcha" 
                 data-sitekey="test_key" 
                 data-finished-callback="testFinishedCallback">
            </div>
        </form>
    `;

    let callbackCalled = false;
    global.window.testFinishedCallback = (widget) => {
        callbackCalled = true;
    };

    const { CaptchaWidget } = await import('../js/widget.js');

    const element = document.querySelector('.private-captcha');
    assert.ok(element, 'Should find captcha element');

    const widget = new CaptchaWidget(element, {
        sitekey: 'aaaaaaaabbbbccccddddeeeeeeeeeeee',
        debug: true
    });

    // Set up event listener
    let eventFired = false;
    element.addEventListener('privatecaptcha:finish', (event) => {
        eventFired = true;
        assert.ok(event.detail.widget, 'Event should include widget in detail');
        assert.strictEqual(event.detail.element, element, 'Event should include element in detail');
    });

    widget.execute();

    // Wait for async operations to complete (assumes zero puzzle mock above)
    await new Promise(resolve => setTimeout(resolve, 150));

    assert.strictEqual(eventFired, true, 'privatecaptcha:finish event should be fired');
    assert.strictEqual(callbackCalled, true, 'Finished callback should be called');

    // Verify solution is available
    const solution = widget.solution();
    assert.ok(solution, 'Widget should have a solution');
    assert.ok(typeof solution === 'string', 'Solution should be a string');

    console.log('✓ Widget execute test passed');
});
