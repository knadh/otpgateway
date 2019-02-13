(function() {
    if(window.hasOwnProperty("OTPGateway")) {
		return false;
    }
    // Get the script's source URL.
    var s = document.querySelector("#otpgateway-script");
    if(!s || !s.src) {
        throw("otpgateway: script is missing the `otpgateway-script` id")
    }
    
    var l = document.createElement("a");
    l.href = s.src;
    var _root = l.protocol + "//" + l.hostname + (l.port ? ":" + l.port : "");
    if(!_root) {
        throw("otpgateway: unable to detect hostname from the script location")
    }

    // Load the CSS.
    var css = document.createElement("link");
    css.rel = "stylesheet";
    css.type = "text/css";
    css.href = _root + "/static/otp.css";
    document.querySelector("head").appendChild(css);
    
    var onCancel = null;

	// Open an inline dialog.
	function modal(url, title) {
        var mod = document.createElement("div");
            mod.setAttribute("id", "otpgateway-modal-wrap");
            mod.innerHTML = "<div id='otpgateway-modal'></div> <div id='otpgateway-frame-spinner' class='otpgateway-spinner'></div> <iframe id='otpgateway-frame'></iframe>";

        // Insert the modal.
        document.querySelector("body").appendChild(mod);

        // Destroy modal on click-out.
        document.querySelector("#otpgateway-modal").onclick = function() {
            if(onCancel) {
                onCancel();
            }
            close();
        };
        
        var fr = document.querySelector("#otpgateway-frame")
        fr.onload = function() {
            var spin = document.querySelector("#otpgateway-frame-spinner");
            if(spin) {
                spin.remove();
            }
        };
        frc = fr.contentDocument;
		frc.open();
		frc.write("<!doctype html><html><head></head><body></body></html>");
		frc.close();
        frc.location = url;
    }
    
    function close() {
        document.querySelector("#otpgateway-modal-wrap").remove();
    }

    window.OTPGateway = function(namespace, id, cbFinish, cbCancel) {
        var win = modal(_root + "/otp/" + namespace + "/" + id + "?view=popup", "Verification");

        // Add a one time event listener for callbacks from the popup if
        // there's a callback.
        if(cbFinish) {
            window.addEventListener("message", (function() {
                function handle(e) {
                    if(e.origin.indexOf(_root) === -1) {
                        return;
                    }
                    removeEventListener(e.type, handle);

                    // Trigger the callback.
                    if(e.data.hasOwnProperty("namespace") && e.data.hasOwnProperty("id")) {
                        cbFinish(e.data.namespace, e.data.id);
                    }
                    close()
                }
                return handle;
            }()));
        }

        onCancel = cbCancel;
    };
})();
