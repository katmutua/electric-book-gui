import { AddNewBookDialog as addNewBookDialog } from './Templates';
import { Eventify } from './Eventify';
import $ = require('jquery');

// AddNewBookDialog steps the user through two pages
// determining what sort of new book they want to create,
// and where the original of that book should be found:
// ie copy the ebw electricbook template, or fork an existing
// book.
export class AddNewBookDialog extends addNewBookDialog {
	constructor(parent:HTMLElement) {
		super();
		Eventify(this.el, {
			'choseType': ()=>{
				let newBook = this.$.newBookRadio.checked;
				let collaborate = this.$.collaborateRadio.checked;
				if (!newBook && !collaborate) {
					alert(`You need to choose one or the other`);
					return;
				}

				if (newBook) {
					this.$.newBook.style.display = 'block';
					this.$.repo_name.focus();
				} else {
					this.$.collaborate.style.display = 'block';
					this.$.collaborate_repo.focus();
				}
				this.$.chooseType.style.display = 'none';
			}
		});

		$(parent).bind(`open.zf.reveal`, (evt)=>{
			this.$.chooseType.style.display = 'block';
			this.$.newBookRadio.checked = false;
			this.$.collaborateRadio.checked = false;

			this.$.newBook.style.display = 'none';
			this.$.repo_name.value = '';

			this.$.collaborate.style.display = 'none';
			this.$.collaborate_repo.value = '';
		});

		parent.appendChild(this.el);
	}
	static instantiate() {
		let list = document.querySelectorAll(`[data-instance='AddNewBookDialog']`);
		for (let i=0; i<list.length; i++) {
			let el = list.item(i) as HTMLElement;
			console.log(`qsa.forEach(`, el, `)`);
			new AddNewBookDialog(el);
		}
	}
}

