(function() {
  "use strict"

  App.Views.LoginPage = function() {
    view.View.call(this, "div")
    this.el.className = "login"

    this.login = this.login.bind(this)
    this.onLoginSuccess = this.onLoginSuccess.bind(this)
    this.onLoginFailure = this.onLoginFailure.bind(this)
  }

  App.Views.LoginPage.prototype = Object.create(view.View.prototype)

  var p = App.Views.LoginPage.prototype
  p.template = JST["templates/login_page"]

  p.listen = function() {
    var form = this.el.querySelector("form")
    form.addEventListener("submit", this.login)
  }

  p.login = function(e) {
    e.preventDefault()
    var form = e.currentTarget
    var credentials = {
      email: form.elements.email.value,
      password: form.elements.password.value
    }
    conn.login(credentials, {
      succeeded: this.onLoginSuccess,
      failed: function(response) { this.onLoginFailure(response.message) }.bind(this)
    })
  }

  p.onLoginSuccess = function(data) {
    localStorage.setItem("sessionID", data.sessionID)

    conn.initChat({
      succeeded: function(data) {
        window.chat = new App.Models.Chat(data)
        window.router.navigate('home')
      }
    })
  }

  p.onLoginFailure = function(response) {
    alert(response)
  }

  p.render = function() {
    this.el.innerHTML = this.template()
    this.listen()
    return this.el
  }
})()
