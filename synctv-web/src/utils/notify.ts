/**
 * @author Lazy
 * @description 弹窗组件
 * 能用，但只能用一点点。纯摆设
 */

class Notify {
  private title!: string;
  private content!: string;
  private status!: string;

  constructor() {
    let notifyBox = document.createElement("div");
    notifyBox.id = "notifyBox";
    document.body.appendChild(notifyBox);
  }

  init(title: string, content: string, status: string) {
    this.title = title;
    this.content = content;
    this.status = status;
    this.show();
  }

  async show() {
    let div = document.createElement("div");
    const title = document.createElement("h2");
    const content = document.createElement("p");
    let nid = Date.now();
    div.className = "notifyyy";
    div.setAttribute("nid", String(nid));
    title.textContent = this.title;
    content.textContent = this.content;
    div.append(title, content);
    document.querySelector("#notifyBox")?.appendChild(div);
    setTimeout(() => {
      document.querySelector(`div[nid="${nid}"]`)?.remove();
    }, 3000);
  }
}

export default Notify;
