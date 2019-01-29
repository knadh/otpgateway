(function() {
    if(window.hasOwnProperty("OTPGateway")) {
		return false;
	}

    var _root = "http://localhost:9000";

    function popup(url, title, w, h) {
        var me = this;

        // Compute the width for the dialog.
        if(!w) {
            w = screen.width/2 < 500 ? 500 : screen.width/2;
        }
        if(!h) {
            h = screen.height/1.5 < 500 ? 500 : screen.height/1.8;
        }

        var	left = (screen.width / 2) - (w/2),
            top = (screen.height / 2) - (h/2);

        var params = "width=%,height=%,left=%,top=%,status=no,menubar=no,toolbar=no,scrollbars=yes"
                        .replace("%", w)
                        .replace("%", h)
                        .replace("%", left)
                        .replace("%", top);

        this.win = window.open(url, title, params);
        return this.win.document;
    }

    window.OTPGateway = function(namespace, id, cb) {
        var win = popup("", "Verification", 475);
        win.location = _root + "/otp/" + namespace + "/" + id + "?view=popup";

        // Add a one time event listener for callbacks from the popup if
        // there's a callback.
        if(!cb) {
            return;
        }
        window.addEventListener("message", (function() {
            function handle(e) {
                if(e.origin.substr(_root) === -1) {
                    return;
                }
                removeEventListener(e.type, handle);

                // Trigger the callback.
                if(e.data.hasOwnProperty("namespace") && e.data.hasOwnProperty("id")) {
                    cb(e.data.namespace, e.data.id);
                }
            }

            return handle;
        }()));
    };
})();
